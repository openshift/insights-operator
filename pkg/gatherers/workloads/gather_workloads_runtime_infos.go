package workloads

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/klog/v2"
)

type podWithNodeName struct {
	podIP    string
	nodeName string
}

func gatherWorkloadRuntimeInfos(
	ctx context.Context,
	coreClient corev1client.CoreV1Interface,
) (workloadRuntimes, []error) {
	start := time.Now()

	runtimePodIPs, err := getInsightsOperatorRuntimePodIPs(ctx, coreClient)
	if err != nil {
		return nil, []error{err}
	}

	workloadRuntimeInfos := make(workloadRuntimes)
	var errors = []error{}

	nodeWorkloadCh := make(chan workloadRuntimesResult)
	var receiveWg sync.WaitGroup
	receiveWg.Add(1)

	go func() {
		defer receiveWg.Done()
		for infosRes := range nodeWorkloadCh {
			if infosRes.Error != nil {
				errors = append(errors, infosRes.Error)
				continue
			}
			mergeWorkloads(workloadRuntimeInfos, infosRes.WorkloadRuntimes)
		}
	}()

	var wg sync.WaitGroup
	wg.Add(len(runtimePodIPs))
	for i := range runtimePodIPs {
		go func(podInfo podWithNodeName) {
			defer wg.Done()
			klog.Infof("Gathering workload runtime info for node %s...\n", podInfo.nodeName)
			hostPort := net.JoinHostPort(podInfo.podIP, "8000")
			extractorURL := fmt.Sprintf("https://%s/gather_runtime_info", hostPort)
			httpCli, err := createHTTPClient()
			if err != nil {
				klog.Errorf("Failed to initialize the HTTP client: %v", err)
				return
			}
			tokenData, err := readToken()
			if err != nil {
				klog.Errorf("Failed to read the serviceaccount token: %v", err)
				return
			}
			nodeWorkloadCh <- getNodeWorkloadRuntimeInfos(ctx, extractorURL, string(tokenData), httpCli)
		}(runtimePodIPs[i])
	}

	wg.Wait()
	close(nodeWorkloadCh)
	receiveWg.Wait()

	klog.Infof("Gathered workload runtime infos in %s\n",
		time.Since(start).Round(time.Second).String())

	return workloadRuntimeInfos, errors
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
	for i := range pods.Items {
		pod := &pods.Items[i]
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

type workloadRuntimesResult struct {
	WorkloadRuntimes workloadRuntimes
	Error            error
}

func createHTTPClient() (*http.Client, error) {
	// Use the certificate authority from the service to verify the TLS connection to the insights-runtime-extractor
	const rootCAFile = "/var/run/configmaps/service-ca-bundle/service-ca.crt"

	caCertPool, err := certutil.NewPool(rootCAFile)
	if err != nil {
		return nil, err
	}

	authClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
				RootCAs:            caCertPool,
				ServerName:         "exporter.openshift-insights.svc.cluster.local",
				MinVersion:         tls.VersionTLS12,
			},
		},
	}
	return authClient, nil
}

func readToken() ([]byte, error) {
	// Read the token for the operator service account
	return os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
}

// Get all WorkloadRuntimeInfos for a single Node (using the insights-runtime-extractor pod running on this node)
func getNodeWorkloadRuntimeInfos(
	ctx context.Context,
	url string,
	token string,
	httpCli *http.Client,
) workloadRuntimesResult {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return workloadRuntimesResult{
			Error: err,
		}
	}
	request.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpCli.Do(request)
	if err != nil {
		return workloadRuntimesResult{
			Error: err,
		}
	}
	if resp.StatusCode != 200 {
		return workloadRuntimesResult{
			Error: insightsclient.HttpError{
				StatusCode: resp.StatusCode,
				Err:        fmt.Errorf("%s", resp.Status),
			},
		}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return workloadRuntimesResult{
			Error: err,
		}
	}
	var nodeOutput nodeRuntimeInfo
	err = json.Unmarshal(body, &nodeOutput)
	if err != nil {
		return workloadRuntimesResult{
			Error: err,
		}
	}

	result := make(workloadRuntimes)

	// Transform the workload data from the insights operator runtime component (represented by
	// a map of namespace/pod-name/container-id/workloadRuntimeInfoContainer) into the data
	// stored by the Insights operator as a map of (namespace/pod-name/container-id) / workloadRuntimeInfoContainer
	for nsName, nsRuntimeInfo := range nodeOutput {
		for podName, podRuntimeInfo := range nsRuntimeInfo {
			for containerID, containerRuntimeInfo := range podRuntimeInfo {
				// skip empty runtime info
				if reflect.DeepEqual(containerRuntimeInfo, workloadRuntimeInfoContainer{}) {
					continue
				}
				cInfo := containerInfo{
					namespace:   nsName,
					pod:         podName,
					containerID: containerID,
				}
				result[cInfo] = containerRuntimeInfo
			}
		}
	}
	return workloadRuntimesResult{
		WorkloadRuntimes: result,
		Error:            err,
	}
}
