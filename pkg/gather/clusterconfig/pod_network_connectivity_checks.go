package clusterconfig

import (
	"context"
	"encoding/json"

	controlplanev1 "github.com/openshift/api/operatorcontrolplane/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherPNCC collects a summary of failed PodNetworkConnectivityChecks.
func GatherPNCC(g *Gatherer, c chan<- gatherResult) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{errors: []error{err}}
		return
	}
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		c <- gatherResult{errors: []error{err}}
		return
	}

	records, errors := gatherPNCC(g.ctx, gatherDynamicClient, gatherKubeClient.CoreV1())
	c <- gatherResult{records: records, errors: errors}
}

func getUnsuccessfulChecks(entries []controlplanev1.LogEntry) []controlplanev1.LogEntry {
	unsuccesseful := []controlplanev1.LogEntry{}
	for _, entry := range entries {
		if !entry.Success {
			unsuccesseful = append(unsuccesseful, entry)
		}
	}
	return unsuccesseful
}

func gatherPNCC(ctx context.Context, dynamicClient dynamic.Interface, coreClient corev1client.CoreV1Interface) ([]record.Record, []error) {
	pnccListUnstruct, err := dynamicClient.Resource(pnccGroupVersionResource).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, []error{err}
	}

	jsonBytes, err := pnccListUnstruct.MarshalJSON()
	if err != nil {
		return nil, []error{err}
	}

	pnccListStruct := controlplanev1.PodNetworkConnectivityCheckList{}
	if err := json.Unmarshal(jsonBytes, &pnccListStruct); err != nil {
		return nil, []error{err}
	}

	unsuccessful := []controlplanev1.LogEntry{}
	for _, pncc := range pnccListStruct.Items {
		unsuccessful = append(unsuccessful, getUnsuccessfulChecks(pncc.Status.Failures)...)
		for _, outage := range pncc.Status.Outages {
			unsuccessful = append(unsuccessful, getUnsuccessfulChecks(outage.StartLogs)...)
			unsuccessful = append(unsuccessful, getUnsuccessfulChecks(outage.EndLogs)...)
		}
	}

	reasons := map[string]map[string]int{}
	for _, entry := range unsuccessful {
		if _, reasonExist := reasons[entry.Reason]; reasonExist {
			msgMap := reasons[entry.Reason]
			if _, msgExist := msgMap[entry.Message]; msgExist {
				msgMap[entry.Message]++
			} else {
				msgMap[entry.Message] = 1
			}
		} else {
			reasons[entry.Reason] = map[string]int{
				entry.Message: 1,
			}
		}
	}

	return []record.Record{{Name: "config/podnetworkconnectivitychecks", Item: record.JSONMarshaller{Object: reasons}}}, nil
}
