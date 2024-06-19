package workloads

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash"
	"os"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog/v2"
)

// Internal representation of workload infor returned by the insights-operator-runtime component.
type insightsWorkloadRuntimeInfo struct {
	OSReleaseID            string             `json:"os-release-id,omitempty"`
	OSReleaseVersionID     string             `json:"os-release-version-id,omitempty"`
	RuntimeKind            string             `json:"runtime-kind,omitempty"`
	RuntimeKindVersion     string             `json:"runtime-kind-version,omitempty"`
	RuntimeKindImplementer string             `json:"runtime-kind-implementer,omitempty"`
	Runtimes               []RuntimeComponent `json:"runtimes,omitempty"`
}

func gatherWorkloadRuntimeInfos(
	ctx context.Context,
	h hash.Hash,
	coreClient corev1client.CoreV1Interface,
	restConfig *rest.Config,
) (workloadRuntimes, error) {
	start := time.Now()

	runtimePods := getInsightsOperatorRuntimePods(coreClient, ctx)

	workloadRuntimeInfos := make(workloadRuntimes)

	nodeWorkloadCh := make(chan workloadRuntimes)
	var wg sync.WaitGroup
	wg.Add(len(runtimePods))

	for nodeName, runtimePodName := range runtimePods {
		go func(nodeName string, runtimePodName string) {
			defer wg.Done()
			klog.Infof("Gathering workload runtime info for node %s...\n", nodeName)
			nodeWorkloadCh <- getNodeWorkloadRuntimeInfos(ctx, h, coreClient, restConfig, runtimePodName)
		}(nodeName, runtimePodName)
	}
	go func() {
		wg.Wait()
		close(nodeWorkloadCh)
	}()

	for infos := range nodeWorkloadCh {
		mergeWorkloads(workloadRuntimeInfos, infos)
	}

	klog.Infof("Gathered workload runtime infos in %s\n",
		time.Since(start).Round(time.Second).String())

	return workloadRuntimeInfos, nil
}

// List the pods of the insights-operator-runtime component
// and returns a map where the key is the name of the worker nodes
// and the value the name of the  insights-operator-runtime's pod running on that worker node.
func getInsightsOperatorRuntimePods(
	coreClient corev1client.CoreV1Interface,
	ctx context.Context,
) map[string]string {
	runtimePods := make(map[string]string)

	pods, err := coreClient.Pods(os.Getenv("POD_NAMESPACE")).
		List(ctx, metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=insights-operator-runtime",
		})
	if err != nil {
		return runtimePods
	}

	for _, pod := range pods.Items {
		runtimePods[pod.Spec.NodeName] = pod.ObjectMeta.Name
	}
	return runtimePods
}

// Merge the workloads from a single node into the global map
func mergeWorkloads(global workloadRuntimes,
	node workloadRuntimes,
) {
	for namespace, nodePodWorkloads := range node {
		if _, exists := global[namespace]; !exists {
			// If the namespace doesn't exist in global, simply assign the value from node.
			global[namespace] = nodePodWorkloads
		} else {
			// If the namespace exists, check the pods
			for podName, containerWorkloads := range nodePodWorkloads {
				if _, exists := global[namespace][podName]; !exists {
					// If the namespace/pod doesn't exist in the global map, assign the value from the node.
					global[namespace][podName] = containerWorkloads
				} else {
					// add the workload from the node
					for containerID, runtimeInfo := range containerWorkloads {
						global[namespace][podName][containerID] = runtimeInfo
					}
				}
			}
		}
	}
}

// Transform the workload data from the insights operator runtime component (represented by
// a map of namespace/pod-name/container-id/insightsWorkloadRuntimeInfo) into the data
// stored by the Insights operator as a map of namespace/pod-name/container-id/workloadRuntimeInfoContainer
// where each value is hashed.
func transformWorkload(h hash.Hash,
	node map[string]map[string]map[string]insightsWorkloadRuntimeInfo,
) workloadRuntimes {

	result := make(workloadRuntimes)

	for podNamespace, podWorkloads := range node {
		result[podNamespace] = make(map[string]map[string]workloadRuntimeInfoContainer)
		for podName, containerWorkloads := range podWorkloads {
			result[podNamespace][podName] = make(map[string]workloadRuntimeInfoContainer)
			for containerID, info := range containerWorkloads {
				runtimeInfo := workloadRuntimeInfoContainer{
					Os:              hashString(h, info.OSReleaseID),
					OsVersion:       hashString(h, info.OSReleaseVersionID),
					Kind:            hashString(h, info.RuntimeKind),
					KindVersion:     hashString(h, info.RuntimeKindVersion),
					KindImplementer: hashString(h, info.RuntimeKindImplementer),
				}

				runtimeInfos := make([]RuntimeComponent, len(info.Runtimes))
				for i, runtime := range info.Runtimes {
					runtimeInfos[i] = RuntimeComponent{
						Name:    hashString(h, runtime.Name),
						Version: hashString(h, runtime.Version),
					}
				}
				runtimeInfo.Runtimes = runtimeInfos
				result[podNamespace][podName][containerID] = runtimeInfo

			}
		}
	}
	return result
}

// hashString return a hash of the string if it is not empty (or the empty string otherwise)
func hashString(h hash.Hash, s string) string {
	if s == "" {
		return s
	}
	return workloadHashString(h, s)
}

// Get all WorkloadRuntimeInfos for a single Node (using the insights-operator-runtime pod running on this node)
func getNodeWorkloadRuntimeInfos(
	ctx context.Context,
	h hash.Hash,
	coreClient corev1client.CoreV1Interface,
	restConfig *rest.Config,
	runtimePodName string,
) workloadRuntimes {
	execCommand := []string{"/scan-containers"}

	req := coreClient.RESTClient().
		Post().
		Namespace(os.Getenv("POD_NAMESPACE")).
		Name(runtimePodName).
		Resource("pods").
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command: execCommand,
			Stdout:  true,
			Stderr:  true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(restConfig, "POST", req.URL())
	if err != nil {
		fmt.Printf("error: %s", err)
	}
	var (
		execOut bytes.Buffer
		execErr bytes.Buffer
	)

	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &execOut,
		Stderr: &execErr,
		Tty:    false,
	})
	if err != nil {
		fmt.Printf("got insights operator runime error: %s\n", err)
		fmt.Printf("command error output: %s\n", execErr.String())
		fmt.Printf("command output: %s\n", execOut.String())
	} else if execErr.Len() > 0 {
		fmt.Printf("command execution got stderr: %s", execErr.String())
	}

	output := execOut.String()

	var nodeOutput map[string]map[string]map[string]insightsWorkloadRuntimeInfo
	json.Unmarshal([]byte(output), &nodeOutput)

	return transformWorkload(h, nodeOutput)
}
