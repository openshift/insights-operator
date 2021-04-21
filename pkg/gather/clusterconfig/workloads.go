package clusterconfig

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"os"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
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

// workloadPods is the top level description of the workloads on the cluster, primarily
// consisting of pod shapes by namespace. The shape of a pod is tracked as the content
// addressable hashes of each container image, a hash of the first command and argument,
// and hashes of the namespace name. This can be used to identify images that are publicly
// available but will not disclose details of private images such as names, content, or
// detailed metadata on the image. All identifying info is required to be hashed before
// sending - values such as "redis" or "/usr/bin/bash" could be reconstructed by comparing
// known hashes for those arguments.
//
// Additions to this data set are required to be reviewed for likelihood of data exposure
// and utility.
type workloadPods struct {
	// PodCount is the count of all pods scanned.
	PodCount int `json:"pods"`
	// ImageCount is the number of unique image IDs identified from pods.
	ImageCount int `json:"imageCount"`
	// Images is a map of image ID to data about the images referenced by pods. Images are
	// only populated if the cluster had imported the image ID to the image API via an
	// import or an image stream.
	Images map[string]workloadImage `json:"images"`
	// Namespaces is a map of namespace name hash to data about the namespace. The namespace
	// is populated even if it has no pods.
	Namespaces map[string]workloadNamespacePods `json:"namespaces"`
}

// workloadImage tracks a minimal set of metadata about images allowing identification
// of parent / child relationships via layers.
type workloadImage struct {
	// LayerIDs is the list of image layers in lowest-to-highest order.
	LayerIDs []string `json:"layerIDs"`
	// FirstCommand is a hash of the first value in the entrypoint array, if
	// any was set. Normalized to be consistent with pods.
	FirstCommand string `json:"firstCommand,omitempty"`
	// FirstArg is a hash of the first value in the command array, if any
	// was set. Normalized to be consistent with pods
	FirstArg string `json:"firstArg,omitempty"`
}

// workloadNamespacePods tracks the identified pod shapes within a namespace.
type workloadNamespacePods struct {
	// Count is the number of pods identified in the namespace.
	Count int `json:"count"`
	// TerminalCount is the number of pods that have reached a terminal phase
	// (success or error) in the namespace.
	TerminalCount int `json:"terminalCount,omitempty"`
	// IgnoredCount is the number of pods that are excluded because they are
	// in the terminal or unknown phases or have no pod status.
	IgnoredCount int `json:"ignoredCount,omitempty"`
	// InvalidCount is the number of pods that are returning partial information
	// about their shapes (no image ID in status) or cannot be evaluated at this
	// time.
	InvalidCount int `json:"invalidCount,omitempty"`
	// Shapes is the identified workload pod shapes in this namespace.
	Shapes []workloadPodShape `json:"shapes"`
}

// workloadPodShape describes a pod shape observed in a namespace. Pod shapes are
// identical if init containers and container shapes are identical.
type workloadPodShape struct {
	// Duplicates is the number of pods that share this shape. The number of
	// pods is always this number + one for the first pod with the shape.
	Duplicates int `json:"duplicates,omitempty"`

	// RestartAlways tracks whether a pod is a service (always restarts) or
	// a job (runs to completion).
	RestartsAlways bool `json:"restartAlways"`
	// InitContainers is the shapes of the init containers in this pod, in
	// the same order as they are defined in spec.
	InitContainers []workloadContainerShape `json:"initContainers,omitempty"`
	// Containers is the shapes of the containers in this pod, in
	// the same order as they are defined in spec.
	Containers []workloadContainerShape `json:"containers"`
}

// workloadContainerShape describes the shape of a container which includes
// a subset of the data in the container.
// TODO: this may desirable to make more precise with a whole container hash
//   that includes more of the workload, but that would only be necessary if
//   it assisted reconstruction of type of workloads.
type workloadContainerShape struct {
	// ImageID is the content addressable hash of the image as observed from
	// the status or the spec tag.
	ImageID string `json:"imageID"`
	// FirstCommand is a hash of the first value in the command array, if
	// any was set.
	FirstCommand string `json:"firstCommand,omitempty"`
	// FirstArg is a hash of the first value in the arguments array, if any
	// was set.
	FirstArg string `json:"firstArg,omitempty"`
}

// GatherWorkloadInfo collects summarized info about the workloads on a cluster
// in a generic fashion
//
// Location in archive: config/workload_info
// Id in config: workload_info
func GatherWorkloadInfo(g *Gatherer, c chan<- gatherResult) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}
	imageConfig := rest.CopyConfig(g.gatherProtoKubeConfig)
	imageConfig.QPS = 10
	imageConfig.Burst = 10
	gatherOpenShiftClient, err := imageclient.NewForConfig(imageConfig)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}
	result, errs := gatherWorkloadInfo(g.ctx, gatherKubeClient.CoreV1(), gatherOpenShiftClient)
	c <- gatherResult{result, errs}
}

func gatherWorkloadInfo(ctx context.Context, coreClient corev1client.CoreV1Interface, imageClient imageclient.ImageV1Interface) ([]record.Record, []error) {
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
			case len(pod.Status.InitContainerStatuses) != len(pod.Spec.InitContainers), len(pod.Status.ContainerStatuses) != len(pod.Spec.Containers):
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
	klog.V(2).Infof("Loaded pods in %s, will wait %s for image data", time.Now().Sub(start).Round(time.Second).String(), waitDuration.Round(time.Second).String())
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
		return records, []error{fmt.Errorf("The %d limit for number of pods gathered was reached", podsLimit)}
	}
	return records, nil
}

type workloadImageInfo struct {
	count  int
	images map[string]workloadImage
}

func gatherWorkloadImageInfo(ctx context.Context, imageClient imageclient.ImageInterface, imageCh <-chan string) <-chan workloadImageInfo {
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
						klog.V(4).Infof("No image %s (%s)", imageID, time.Now().Sub(start).Round(time.Millisecond).String())
						continue
					}
					if err == context.Canceled {
						return
					}
					if err != nil {
						klog.Errorf("Unable to retrieve image %s", imageID)
						continue
					}
					klog.V(4).Infof("Found image %s (%s)", imageID, time.Now().Sub(start).Round(time.Millisecond).String())
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
			return -1
		}
		if !workloadContainerShapesEqual(existing.Containers, shape.Containers) {
			return -1
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
	} else {
		if i := strings.LastIndex(s, "\\"); i != -1 {
			s = s[i+1:]
		}
	}
	return s
}

// Empty returns true if the image has no contents and can be ignored.
func (i workloadImage) Empty() bool {
	return len(i.LayerIDs) == 0
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

func shorterImageID(s string) string {
	if strings.HasPrefix(s, "sha256:") {
		b, err := hex.DecodeString(s[7:])
		if err != nil {
			return s
		}
		return base64.RawURLEncoding.EncodeToString(b)
	}
	return s
}

// calculateWorkloadContainerShapes takes a spec and status slice and attempts
// to calculate an array of container shapes. If the preconditions of the shape
// can't be met (invalid status, no imageID) false is returned.
func calculateWorkloadContainerShapes(h hash.Hash, spec []corev1.Container, status []corev1.ContainerStatus) ([]workloadContainerShape, bool) {
	shapes := make([]workloadContainerShape, 0, len(status))
	for i, container := range status {
		specIndex := matchingSpecIndex(container.Name, spec, i)
		if specIndex == -1 {
			// no matching spec, skip
			fmt.Fprintf(os.Stderr, "warning: unable to match %s to a container spec\n", container.Name)
			return nil, false
		}

		imageID := idForImageReference(strings.TrimPrefix(container.ImageID, "docker-pullable://"))
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
			// TODO: create an example that shows these in unhashed form
			//fmt.Fprintf(os.Stderr, "info: Convert command[0] %q to %q to %q\n", spec[specIndex].Command[0], short, shortHash)
		}
		if len(spec[specIndex].Args) > 0 {
			short := workloadArgumentString(spec[specIndex].Args[0])
			shortHash := workloadHashString(h, short)
			firstArg = shortHash
			//fmt.Fprintf(os.Stderr, "info: Convert arg[0] %q to %q to %q\n", spec[specIndex].Args[0], short, shortHash)
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
