package workloads

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"
)

type podWithNodeName struct {
	podIP    string
	nodeName string
}

func gatherWorkloadRuntimeInfos(
	ctx context.Context,
	coreClient corev1client.CoreV1Interface,
) (workloadRuntimes, error) {
	start := time.Now()

	runtimePodIPs, err := getInsightsOperatorRuntimePodIPs(ctx, coreClient)
	if err != nil {
		return nil, err
	}

	workloadRuntimeInfos := make(workloadRuntimes)

	nodeWorkloadCh := make(chan workloadRuntimes)
	var receiveWg sync.WaitGroup
	receiveWg.Add(1)

	go func() {
		defer receiveWg.Done()
		for infos := range nodeWorkloadCh {
			mergeWorkloads(workloadRuntimeInfos, infos)
		}
	}()

	var wg sync.WaitGroup
	wg.Add(len(runtimePodIPs))
	for i := range runtimePodIPs {
		go func(podInfo podWithNodeName) {
			defer wg.Done()
			klog.Infof("Gathering workload runtime info for node %s...\n", podInfo.nodeName)
			nodeWorkloadCh <- getNodeWorkloadRuntimeInfos(ctx, podInfo.podIP)
		}(runtimePodIPs[i])
	}

	wg.Wait()
	close(nodeWorkloadCh)
	receiveWg.Wait()

	klog.Infof("Gathered workload runtime infos in %s\n",
		time.Since(start).Round(time.Second).String())

	return workloadRuntimeInfos, nil
}

// List the pods of the insights-runtime-extractor component
// and returns a map where the key is the name of the worker nodes
// and the value the IP address of the  insights-runtime-extractor's pod running on that worker node.
func getInsightsOperatorRuntimePodIPs(
	ctx context.Context,
	coreClient corev1client.CoreV1Interface,
) ([]podWithNodeName, error) {
	pods, err := coreClient.Pods(os.Getenv("POD_NAMESPACE")).
		List(ctx, metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=insights-runtime-extractor",
		})
	if err != nil {
		return nil, err
	}

	var runtimePods []podWithNodeName
	for _, pod := range pods.Items {
		running := pod.Status.Phase == corev1.PodRunning
		if running {
			runtimePods = append(runtimePods, podWithNodeName{
				podIP:    pod.Status.PodIP,
				nodeName: pod.Spec.NodeName,
			})
		}
	}
	return runtimePods, nil
}

// Merge the workloads from a single node into the global map
func mergeWorkloads(global workloadRuntimes,
	node workloadRuntimes,
) {
	for cInfo, cRuntimeInfo := range node {
		global[cInfo] = cRuntimeInfo
	}
}

// Get all WorkloadRuntimeInfos for a single Node (using the insights-runtime-extractor pod running on this node)
// FIXME return an (workloadRuntimes, error)
func getNodeWorkloadRuntimeInfos(
	ctx context.Context,
	runtimePodIP string,
) workloadRuntimes {

	extractorURL := fmt.Sprintf("http://%s:8000/gather_runtime_info", runtimePodIP)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "GET", extractorURL, nil)
	if err != nil {
		fmt.Printf("Failed to create request: %v\n", err)
		return nil
	}
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		fmt.Printf("Failed to perform request: %v\n", err)
		return nil
	}
	if resp.StatusCode != 200 {
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Failed to read response body: %v\n", err)
		return nil
	}
	var nodeOutput nodeRuntimeInfo
	json.Unmarshal(body, &nodeOutput)

	result := make(workloadRuntimes)

	// Transform the workload data from the insights operator runtime component (represented by
	// a map of namespace/pod-name/container-id/workloadRuntimeInfoContainer) into the data
	// stored by the Insights operator as a map of (namespace/pod-name/container-id) / workloadRuntimeInfoContainer
	for nsName, nsRuntimeInfo := range nodeOutput {
		for podName, podRuntimeInfo := range nsRuntimeInfo {
			for containerId, containerRuntimeInfo := range podRuntimeInfo {
				cInfo := containerInfo{
					namespace:   nsName,
					pod:         podName,
					containerID: containerId,
				}
				result[cInfo] = containerRuntimeInfo
			}
		}
	}
	return result
}
