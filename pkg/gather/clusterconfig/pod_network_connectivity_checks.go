package clusterconfig

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherPNCC collects PodNetworkConnectivityChecks.
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

func gatherPNCC(ctx context.Context, dynamicClient dynamic.Interface, coreClient corev1client.CoreV1Interface) ([]record.Record, []error) {
	pnccList, err := dynamicClient.Resource(pnccGroupVersionResource).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, []error{err}
	}

	return []record.Record{{Name: "config/podnetworkconnectivitychecks", Item: record.JSONMarshaller{Object: pnccList}}}, nil
}
