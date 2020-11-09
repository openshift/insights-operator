package clusterconfig

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	_ "k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	"github.com/openshift/insights-operator/pkg/record"
)

const (
	// imageGatherPodLimit is the maximum number of pods that
	// will be listed in a single request to reduce memory usage.
	imageGatherPodLimit = 200
)

// GatherContainerImages collects essential information about running containers.
// Specifically, the age of pods, the set of running images and the container names are collected.
//
// Location in archive: config/running_containers.json
func GatherContainerImages(g *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		records := []record.Record{}

		contInfo := ContainerInfo{
			Images:     ContainerImageSet{},
			Containers: PodsWithAge{},
		}
		// Cache for image indices in the collected set of container image.
		image2idx := map[string]int{}

		// Use the Limit and Continue fields to request the pod information in chunks.
		continueValue := ""
		for {
			pods, err := g.coreClient.Pods("").List(g.ctx, metav1.ListOptions{
				Limit:    imageGatherPodLimit,
				Continue: continueValue,
				// FieldSelector: "status.phase=Running",
			})
			if err != nil {
				return nil, []error{err}
			}

			for podIndex, pod := range pods.Items {
				podPtr := &pods.Items[podIndex]
				if strings.HasPrefix(pod.Namespace, "openshift") && hasContainerInCrashloop(podPtr) {
					records = append(records, record.Record{Name: fmt.Sprintf("config/pod/%s/%s", pod.Namespace, pod.Name), Item: PodAnonymizer{podPtr}})
				} else if pod.Status.Phase == corev1.PodRunning {
					for _, container := range pod.Spec.Containers {
						imageURL := container.Image
						urlHost, err := forceParseURLHost(imageURL)
						if err != nil {
							klog.Errorf("unable to parse container image URL: %v", err)
							continue
						}
						if imageHostRegex.MatchString(urlHost) {
							imgIndex, ok := image2idx[imageURL]
							if !ok {
								imgIndex = contInfo.Images.Add(imageURL)
								image2idx[imageURL] = imgIndex
							}
							contInfo.Containers.Add(pod.CreationTimestamp.Time, imgIndex)
						}
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

		return append(records, record.Record{
			Name: "config/running_containers",
			Item: record.JSONMarshaller{Object: contInfo},
		}), nil
	}
}

// RunningImages assigns information about running containers to a specific image index.
// The index is a reference to an item in the related `ContainerImageSet` instance.
type RunningImages map[int]int

// PodsWithAge maps the YYYY-MM string representation of start time to list of pods running since that month.
type PodsWithAge map[string]RunningImages

// Add inserts the specified container information into the data structure.
func (p PodsWithAge) Add(startTime time.Time, image int) {
	const YyyyMmFormat = "2006-01"
	month := startTime.UTC().Format(YyyyMmFormat)
	if imageMap, exists := p[month]; exists {
		if _, exists := imageMap[image]; exists {
			imageMap[image]++
		} else {
			imageMap[image] = 1
		}
	} else {
		p[month] = RunningImages{image: 1}
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
