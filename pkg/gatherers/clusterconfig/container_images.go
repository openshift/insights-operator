package clusterconfig

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/check"
	"github.com/openshift/library-go/pkg/image/reference"
)

const (
	// imageGatherPodLimit is the maximum number of pods that
	// will be listed in a single request to reduce memory usage.
	imageGatherPodLimit = 200

	// containerImageLimit is the maximum number of container images to collect.
	// On average, information about one image takes up roughly 100 raw bytes.
	containerImageLimit = 1000

	// yyyyMmDateFormat is the date format used to get a YYYY-MM string.
	yyyyMmDateFormat = "2006-01"
)

// GatherContainerImages collects essential information about running containers.
// Specifically, the age of pods, the set of running images and the container names are collected.
//
// * Location in archive: config/running_containers.json
// * Id in config: container_images
// * Since versions:
//   * 4.5.33+
//   * 4.6.16+
//   * 4.7+
func (g *Gatherer) GatherContainerImages(ctx context.Context) ([]record.Record, []error) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}
	return gatherContainerImages(ctx, gatherKubeClient.CoreV1())
}

func gatherContainerImages(ctx context.Context, coreClient corev1client.CoreV1Interface) ([]record.Record, []error) {
	var records []record.Record

	// Cache for the temporary image count list.
	img2month2count := img2Month2CountMap{}

	// Use the Limit and Continue fields to request the pod information in chunks.
	continueValue := ""
	for {
		pods, err := coreClient.Pods("").List(ctx, metav1.ListOptions{
			Limit:    imageGatherPodLimit,
			Continue: continueValue,
			// FieldSelector: "status.phase=Running",
		})
		if err != nil {
			return nil, []error{err}
		}

		for podIndex, pod := range pods.Items { //nolint:gocritic
			podPtr := &pods.Items[podIndex]
			if strings.HasPrefix(pod.Namespace, "openshift-") && check.HasContainerInCrashloop(podPtr) {
				records = append(records, record.Record{
					Name: fmt.Sprintf("config/pod/%s/%s", pod.Namespace, pod.Name),
					Item: record.ResourceMarshaller{Resource: podPtr},
				})
			} else if pod.Status.Phase == corev1.PodRunning {
				startMonth := pod.CreationTimestamp.Time.UTC().Format(yyyyMmDateFormat)

				gatherImages(startMonth, img2month2count, pod.Status.ContainerStatuses)
				gatherImages(startMonth, img2month2count, pod.Status.InitContainerStatuses)
				gatherImages(startMonth, img2month2count, pod.Status.EphemeralContainerStatuses)
			}
		}

		// If the Continue field is not set, this should be the end of available data.
		// Otherwise, update the Continue value and perform another request iteration.
		if pods.Continue == "" {
			break
		}
		continueValue = pods.Continue
	}

	// Transform map into a list for sorting.
	var imageCounts []tmpImageCountEntry
	for img, countMap := range img2month2count {
		totalCount := 0
		for _, count := range countMap {
			totalCount += count
		}
		imageCounts = append(imageCounts, tmpImageCountEntry{
			Image:         img,
			TotalCount:    totalCount,
			CountPerMonth: countMap,
		})
	}

	// Sort images from most common to least common.
	sort.Slice(imageCounts, func(i, j int) bool {
		return imageCounts[i].TotalCount > imageCounts[j].TotalCount
	})

	// Reconstruct the image information into the reported data structure.
	contInfo := ContainerInfo{
		Images:     ContainerImageSet{},
		Containers: PodsWithAge{},
	}
	totalEntries := 0
	for _, img := range imageCounts {
		if totalEntries >= containerImageLimit {
			break
		}

		imgIndex := contInfo.Images.Add(img.Image)
		for month, count := range img.CountPerMonth {
			contInfo.Containers.Add(month, imgIndex, count)
			totalEntries++
		}
	}

	return append(records, record.Record{
		Name: "config/running_containers",
		Item: record.JSONMarshaller{Object: contInfo},
	}), nil
}

// RunningImages assigns information about running containers to a specific image index.
// The index is a reference to an item in the related `ContainerImageSet` instance.
type RunningImages map[int]int

// PodsWithAge maps the YYYY-MM string representation of start time to list of pods running since that month.
type PodsWithAge map[string]RunningImages

// Add inserts the specified container information into the data structure.
func (p PodsWithAge) Add(startMonth string, image, count int) {
	if imageMap, exists := p[startMonth]; exists {
		imageMap[image] += count
	} else {
		p[startMonth] = RunningImages{image: count}
	}
}

// ContainerImageSet is used to store unique container image URLs.
// The key is a continuous index starting from 0.
// The value is the image URL itself.
type ContainerImageSet map[int]string

// Add puts the image at the end of the set.
// It will be assigned the highest index and this index will be returned.
func (is ContainerImageSet) Add(image string) int {
	nextIndex := len(is)
	is[nextIndex] = image
	return nextIndex
}

// ContainerInfo encapsulates the essential information about running containers in a minimalized data structure.
type ContainerInfo struct {
	Images     ContainerImageSet `json:"images"`
	Containers PodsWithAge       `json:"containers"`
}

type img2Month2CountMap map[string]map[string]int

type tmpImageCountEntry struct {
	Image         string
	CountPerMonth map[string]int
	TotalCount    int
}

func gatherImages(startMonth string, img2month2count img2Month2CountMap, containers []corev1.ContainerStatus) {
	for _, container := range containers { //nolint:gocritic
		dockerRef, err := reference.Parse(container.Image)
		if err != nil {
			klog.Warningf("Unable to parse container image specification: %v", err)
			continue
		}

		// Use the sha256 hash ID if available, otherwise use the full image spec.
		imgMinimal := dockerRef.ID
		if imgMinimal == "" {
			imgMinimal = container.Image
		}

		if countMap, ok := img2month2count[imgMinimal]; ok {
			var count int
			if count, ok = countMap[startMonth]; !ok {
				count = 0
			}
			countMap[startMonth] = count + 1
		} else {
			img2month2count[imgMinimal] = map[string]int{
				startMonth: 1,
			}
		}
	}
}
