// nolint: dupl
package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherContainerRuntimeConfig Collects ContainerRuntimeConfig information.
//
// ### API Reference
// - https://github.com/openshift/machine-config-operator/blob/master/pkg/apis/machineconfiguration.openshift.io/v1/types.go#L402
// - https://docs.okd.io/latest/rest_api/machine_apis/containerruntimeconfig-machineconfiguration-openshift-io-v1.html
//
// ### Sample data
// - docs/insights-archive-sample/config/containerruntimeconfigs/set-log-and-pid.json
//
// ### Location in archive
// | Version   | Path														|
// | --------- | --------------------------------------------------------	|
// | >= 4.7.0  | config/containerruntimeconfigs/{name}.json	    			|
//
// ### Config ID
// `clusterconfig/container_runtime_configs`
//
// ### Released version
// - 4.7.0
//
// ### Backported versions
// - 4.6.18+
//
// ### Notes
// None
func (g *Gatherer) GatherContainerRuntimeConfig(ctx context.Context) ([]record.Record, []error) {
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherContainerRuntimeConfig(ctx, dynamicClient)
}

func gatherContainerRuntimeConfig(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	crc := schema.GroupVersionResource{Group: "machineconfiguration.openshift.io", Version: "v1", Resource: "containerruntimeconfigs"}
	containerRCs, err := dynamicClient.Resource(crc).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	var records []record.Record
	for i, crc := range containerRCs.Items {
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/containerruntimeconfigs/%s", crc.GetName()),
			Item: record.ResourceMarshaller{Resource: &containerRCs.Items[i]},
		})
	}
	return records, nil
}
