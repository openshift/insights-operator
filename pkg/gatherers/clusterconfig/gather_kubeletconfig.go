package clusterconfig

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/record"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

// GatherKubeletConfig Collects definitions of Kubeletconfigs
//
// ### API Reference
// - https://docs.redhat.com/en/documentation/openshift_container_platform/4.21/html/machine_apis/kubeletconfig-machineconfiguration-openshift-io-v1
//
// ### Sample data
// - docs/insights-archive-sample/config/kubeletconfigs/set-max-pods.json
//
// ### Location in archive
// - `config/kubeletconfigs/{name}.json`
//
// ### Config ID
// `clusterconfig/kubeletconfigs`
//
// ### Released version
// - 4.22.0
//
// ### Backported versions
//
// ### Changes
// None
func (g *Gatherer) GatherKubeletConfig(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}
	return gatherGatherKubeletConfig(ctx, gatherDynamicClient)
}

func gatherGatherKubeletConfig(ctx context.Context, client dynamic.Interface) ([]record.Record, []error) {
	kubeletConfigList, err := client.Resource(kubeletGroupVersionResource).List(ctx, v1.ListOptions{})
	if errors.IsNotFound(err) {
		klog.Errorf("GatherKubeletConfig: Kubeletconfigs resource not found in cluster (may not be created)")
		return nil, nil
	}
	if err != nil {
		klog.Errorf("GatherKubeletConfig: Failed to list Kubeletconfigs")
		return nil, []error{err}
	}

	var records []record.Record

	for _, kubeletconfig := range kubeletConfigList.Items {
		recordName := fmt.Sprintf("config/kubeletconfigs/%s",
			kubeletconfig.GetName(),
		)

		records = append(records, record.Record{
			Name: recordName,
			Item: record.JSONMarshaller{Object: kubeletconfig.Object},
		})
	}

	return records, nil
}
