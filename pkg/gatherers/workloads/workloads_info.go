package workloads

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"hash"
	"os"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/golang/groupcache/lru"
	"github.com/openshift/api/image/docker10"
	imagev1 "github.com/openshift/api/image/v1"
	imageclient "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"
	"github.com/openshift/library-go/pkg/image/imageutil"
	"github.com/openshift/library-go/pkg/image/reference"

	"github.com/openshift/insights-operator/pkg/record"
)

const (
	// workloadGatherPageSize is 500 (the default for Kube).
	workloadGatherPageSize = 500
	// limit the number of collected Pods in this gatherer. In the worst case, one Pod can add around 600 bytes (before compression)
	// This limit can be removed in the future.
	podsLimit = 3000
)

// GatherWorkloadInfo collects summarized info about the workloads on a cluster
// in a generic fashion
//
// * Location in archive: config/workload_info
// * Id in config: workload_info
// * Since versions:
//   * 4.8+
func (g *Gatherer) GatherWorkloadInfo(ctx context.Context) ([]record.Record, []error) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	imageConfig := rest.CopyConfig(g.gatherProtoKubeConfig)
	imageConfig.QPS = 10
	imageConfig.Burst = 10

	gatherOpenShiftClient, err := imageclient.NewForConfig(imageConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherWorkloadInfo(ctx, gatherKubeClient.CoreV1(), gatherOpenShiftClient)
}

//nolint: funlen, gocyclo, gocritic
func gatherWorkloadInfo(
	ctx context.Context,
	coreClient corev1client.CoreV1Interface,
	imageClient imageclient.ImageV1Interface,
) ([]record.Record, []error) {
	// load images as we find them
	imageCh := make(chan string, workloadGatherPageSize)
	imagesDoneCh := gatherWorkloadImageInfo(ctx, imageClient.Images(), imageCh)

	// load pods in order
	start := time.Now()
	limitReached := false

	var info workloadPods
	var namespace string
	var namespaceHash string
	var namespacePods workloadNamespacePods
	h := sha256.New()

	// Use the Limit and Continue fields to request the pod information in chunks.
	var continueValue string
	for {
		pods, err := coreClient.Pods("").List(ctx, metav1.ListOptions{
			Limit:    workloadGatherPageSize,
			Continue: continueValue,
		})
		if err != nil {
			return nil, []error{err}
		}
		for _, pod := range pods.Items {
			// initialize the running state, including the namespace hash
			if pod.Namespace != namespace {
				if len(namespace) != 0 {
					if info.Namespaces == nil {
						info.Namespaces = make(map[string]workloadNamespacePods, 1024)
					}
					info.Namespaces[namespaceHash] = namespacePods
					info.PodCount += namespacePods.Count
				}
				namespace = pod.Namespace
				namespaceHash = workloadHashString(h, namespace)
				namespacePods = workloadNamespacePods{Shapes: make([]workloadPodShape, 0, 16)}
			}
			// we also need to check the number of pods in current namespace, because when
			// there's a namespace with a lot of pods it could exceed the limit a lot
			if info.PodCount >= podsLimit || info.PodCount+namespacePods.Count >= podsLimit {
				pods.Continue = ""
				limitReached = true
				break
			}
			namespacePods.Count++

			switch {
			case pod.Status.Phase == corev1.PodSucceeded, pod.Status.Phase == corev1.PodFailed:
				// track terminal pods but do not report their data
				namespacePods.TerminalCount++
				continue
			case pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending:
				// only consider pods that are in a known state
				namespacePods.IgnoredCount++
				continue
			case len(pod.Status.InitContainerStatuses) != len(pod.Spec.InitContainers),
				len(pod.Status.ContainerStatuses) != len(pod.Spec.Containers):
				// pods without filled out status are invalid
				namespacePods.IgnoredCount++
				continue
			}

			var podShape workloadPodShape
			var ok bool
			podShape.InitContainers, ok = calculateWorkloadContainerShapes(h, pod.Spec.InitContainers, pod.Status.InitContainerStatuses)
			if !ok {
				namespacePods.InvalidCount++
				continue
			}
			podShape.Containers, ok = calculateWorkloadContainerShapes(h, pod.Spec.Containers, pod.Status.ContainerStatuses)
			if !ok {
				namespacePods.InvalidCount++
				continue
			}

			podShape.RestartsAlways = pod.Spec.RestartPolicy == corev1.RestartPolicyAlways

			if index := workloadPodShapeIndex(namespacePods.Shapes, podShape); index != -1 {
				namespacePods.Shapes[index].Duplicates++
			} else {
				namespacePods.Shapes = append(namespacePods.Shapes, podShape)

				for _, container := range podShape.InitContainers {
					imageCh <- container.ImageID
				}
				for _, container := range podShape.Containers {
					imageCh <- container.ImageID
				}
			}
		}

		// If the Continue field is not set, this should be the end of available data.
		// Otherwise, update the Continue value and perform another request iteration.
		if pods.Continue == "" {
			break
		}
		continueValue = pods.Continue
	}
	// add the last set of pods
	if len(namespace) != 0 {
		if info.Namespaces == nil {
			info.Namespaces = make(map[string]workloadNamespacePods, 1)
		}
		info.Namespaces[namespaceHash] = namespacePods
		info.PodCount += namespacePods.Count
	}

	workloadImageResize(info.PodCount)

	records := []record.Record{
		{
			Name: "config/workload_info",
			Item: record.JSONMarshaller{Object: &info},
		},
	}

	// wait for as many images as we can find to load
	var imageInfo workloadImageInfo
	// wait proportional to the number of pods + a floor
	waitDuration := time.Second*time.Duration(info.PodCount)/10 + 15*time.Second
	klog.V(2).Infof("Loaded pods in %s, will wait %s for image data",
		time.Since(start).Round(time.Second).String(),
		waitDuration.Round(time.Second).String())
	close(imageCh)
	select {
	case <-ctx.Done():
		select {
		case imageInfo = <-imagesDoneCh:
			// we can use the loaded images
		case <-time.After(10 * time.Second):
			// we can't use any of the loaded images
		}
	case imageInfo = <-imagesDoneCh:
		// we can use the loaded images
	case <-time.After(waitDuration):
		// we can't use any of the loaded images
	}

	info.Images = imageInfo.images
	info.ImageCount = imageInfo.count
	if limitReached {
		return records, []error{fmt.Errorf("the %d limit for number of pods gathered was reached", podsLimit)}
	}
	return records, nil
}

//nolint: gocyclo
func gatherWorkloadImageInfo(
	ctx context.Context,
	imageClient imageclient.ImageInterface,
	imageCh <-chan string,
) <-chan workloadImageInfo {
	images := make(map[string]workloadImage)
	imagesDoneCh := make(chan workloadImageInfo)

	go func() {
		h := sha256.New()

		defer func() {
			count := len(images)
			for k, v := range images {
				if v.Empty() {
					delete(images, k)
				}
			}
			imagesDoneCh <- workloadImageInfo{
				count:  count,
				images: images,
			}
			close(imagesDoneCh)
		}()
		doneCh := ctx.Done()
		pendingIDs := make(map[string]struct{}, workloadGatherPageSize)
		for {
			select {
			case <-doneCh:
				return
			case imageID, ok := <-imageCh:
				if !ok {
					return
				}

				// drain the channel of any image IDs
				for k := range pendingIDs {
					delete(pendingIDs, k)
				}
				if _, ok := images[imageID]; !ok {
					pendingIDs[imageID] = struct{}{}
				}
				for l := len(imageCh); l > 0; l = len(imageCh) {
					for i := 0; i < l; i++ {
						imageID := <-imageCh
						if _, ok := images[imageID]; !ok {
							pendingIDs[imageID] = struct{}{}
						}
					}
				}

				for imageID := range pendingIDs {
					if _, ok := images[imageID]; ok {
						continue
					}
					if image, ok := workloadImageGet(imageID); ok {
						images[imageID] = image
						continue
					}
					images[imageID] = workloadImage{}
					start := time.Now()
					image, err := imageClient.Get(ctx, imageID, metav1.GetOptions{})
					if errors.IsNotFound(err) {
						klog.V(4).Infof("No image %s (%s)", imageID, time.Since(start).Round(time.Millisecond).String())
						continue
					}
					if err == context.Canceled {
						return
					}
					if err != nil {
						klog.Errorf("Unable to retrieve image %s", imageID)
						continue
					}
					klog.V(4).Infof("Found image %s (%s)", imageID, time.Since(start).Round(time.Millisecond).String())
					info := calculateWorkloadInfo(h, image)
					images[imageID] = info
					workloadImageAdd(imageID, info)
				}
			}
		}
	}()
	return imagesDoneCh
}

// workloadPodShapeIndex attempts to find an equivalent shape within the current
// namespace, returning the index of the matching shape or -1. It exploits the
// property that identical pods tend to have similar name prefixes and searches
// in reverse order from the most recent shape (since pods appear in name order).
// TODO: some optimization in very large namespaces with diverse shapes may be
//   necessary
func workloadPodShapeIndex(shapes []workloadPodShape, shape workloadPodShape) int {
	for i := len(shapes) - 1; i >= 0; i-- {
		existing := shapes[i]
		if !workloadContainerShapesEqual(existing.InitContainers, shape.InitContainers) {
			continue
		}
		if !workloadContainerShapesEqual(existing.Containers, shape.Containers) {
			continue
		}
		return i
	}
	return -1
}

// workloadContainerShapesEqual returns true if the provided shape arrays are equal
func workloadContainerShapesEqual(a, b []workloadContainerShape) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// workloadHashString returns a base64 URL encoded version of the hash of the
// provided string. The resulting string length of 12 is chosen to have a
// probability of a collision across 1 billion results of 0.0001.
func workloadHashString(h hash.Hash, s string) string {
	h.Reset()
	h.Write([]byte(s))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))[:12]
}

func workloadArgumentString(s string) string {
	// if this is a multipart script string, split it to get the first
	// part
	s = strings.TrimSpace(s)
	if i := strings.Index(s, " "); i != -1 {
		s = s[:i]
	}
	// if this looks like a flag, strip any possible value to ensure we
	// don't accidentally gather a user value
	if strings.HasPrefix(s, "-") {
		i := strings.Index(s, "=")
		if i == -1 {
			// skip
			return ""
		}
		s = s[:i]
	}
	// if this string contains slashes, grab the end of the string, i.e.
	// /usr/bin/bash -> bash, c:\foo\bar -> bar
	s = strings.Trim(s, "/\\")
	if i := strings.LastIndex(s, "/"); i != -1 {
		s = s[i+1:]
	} else if i := strings.LastIndex(s, "\\"); i != -1 {
		s = s[i+1:]
	}
	return s
}

// idForImageReference attempts to retrieve the image ID from the provided
// image reference or returns the empty string if no such ID is found.
func idForImageReference(s string) string {
	if len(s) == 0 {
		return ""
	}
	ref, err := reference.Parse(s)
	if err != nil {
		return ""
	}
	return ref.ID
}

// calculateWorkloadContainerShapes takes a spec and status slice and attempts
// to calculate an array of container shapes. If the preconditions of the shape
// can't be met (invalid status, no imageID) false is returned.
func calculateWorkloadContainerShapes(
	h hash.Hash,
	spec []corev1.Container,
	status []corev1.ContainerStatus,
) ([]workloadContainerShape, bool) {
	shapes := make([]workloadContainerShape, 0, len(status))
	for i := range status {
		specIndex := matchingSpecIndex(status[i].Name, spec, i)
		if specIndex == -1 {
			// no matching spec, skip
			fmt.Fprintf(os.Stderr, "warning: unable to match %s to a container spec\n", status[i].Name)
			return nil, false
		}

		imageID := idForImageReference(strings.TrimPrefix(status[i].ImageID, "docker-pullable://"))
		if len(imageID) == 0 {
			imageID = idForImageReference(spec[specIndex].Image)
		}
		if len(imageID) == 0 {
			return nil, false
		}

		var firstCommand, firstArg string
		if len(spec[specIndex].Command) > 0 {
			short := workloadArgumentString(spec[specIndex].Command[0])
			shortHash := workloadHashString(h, short)
			firstCommand = shortHash
		}
		if len(spec[specIndex].Args) > 0 {
			short := workloadArgumentString(spec[specIndex].Args[0])
			shortHash := workloadHashString(h, short)
			firstArg = shortHash
		}

		shapes = append(shapes, workloadContainerShape{
			ImageID:      imageID,
			FirstCommand: firstCommand,
			FirstArg:     firstArg,
		})
	}
	return shapes, true
}

// calculateWorkloadInfo converts an image object into the minimal info we
// recover. If the image can't be converted we only gather layer data.
func calculateWorkloadInfo(h hash.Hash, image *imagev1.Image) workloadImage {
	layers := make([]string, 0, len(image.DockerImageLayers))
	for _, layer := range image.DockerImageLayers {
		layers = append(layers, layer.Name)
	}
	info := workloadImage{
		LayerIDs: layers,
	}

	if err := imageutil.ImageWithMetadata(image); err != nil {
		return info
	}
	if image.DockerImageMetadata.Object == nil {
		return info
	}
	imageMeta, ok := image.DockerImageMetadata.Object.(*docker10.DockerImage)
	if !ok {
		return info
	}

	if len(imageMeta.ContainerConfig.Entrypoint) > 0 {
		short := workloadArgumentString(imageMeta.ContainerConfig.Entrypoint[0])
		shortHash := workloadHashString(h, short)
		info.FirstCommand = shortHash
	}
	if len(imageMeta.ContainerConfig.Cmd) > 0 {
		short := workloadArgumentString(imageMeta.ContainerConfig.Cmd[0])
		shortHash := workloadHashString(h, short)
		info.FirstArg = shortHash
	}

	return info
}

// matchingSpecIndex attempts to find the index of the named container within
// the spec array, starting with the value of hint. Kubelets are required to
// set container status to match the order of spec containers but may fail to
// do so in some scenarios, so guard against inconsistency.
func matchingSpecIndex(name string, spec []corev1.Container, hint int) int {
	if hint < len(spec) {
		if spec[hint].Name == name {
			return hint
		}
	}
	for i := range spec {
		if spec[i].Name == name {
			return i
		}
	}
	return -1
}

var (
	workloadSizeLock sync.Mutex
	workloadImageLRU = lru.New(workloadGatherPageSize)
)

// workloadImageResize resizes the image LRU cache to match the rough number
// of pods on the cluster. This allows old images to expire from the cache
// over very long periods of time without having to flush the cache.
func workloadImageResize(estimatedSize int) {
	workloadSizeLock.Lock()
	defer workloadSizeLock.Unlock()
	workloadImageLRU.MaxEntries = int(float64(estimatedSize) * float64(1.2))
}

// workloadImageGet returns the cached image if it is found or false.
func workloadImageGet(imageID string) (workloadImage, bool) {
	workloadSizeLock.Lock()
	defer workloadSizeLock.Unlock()
	v, ok := workloadImageLRU.Get(imageID)
	if !ok {
		return workloadImage{}, false
	}
	return v.(workloadImage), true
}

// workloadImageAdd adds the provided image to the cache.
func workloadImageAdd(imageID string, image workloadImage) {
	workloadSizeLock.Lock()
	defer workloadSizeLock.Unlock()
	workloadImageLRU.Add(imageID, image)
}
