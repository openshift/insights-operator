package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	_ "k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	"github.com/openshift/insights-operator/pkg/record"
)

const sdnNamespace = "openshift-sdn"
const ovsCommand = "ovs-ofctl -O OpenFlow13 dump-flows br0"

// GatherOVSFlows collects OVS flow information for every OVS pod in the openshift-sdn namespace
//
// Location in archive: networking/ovs_flows/
// Id in config: ovs_flows
func GatherOVSFlows(g *Gatherer) ([]record.Record, []error) {
	return gatherOVSFlows(g.ctx, g.gatherProtoKubeConfig)
}

func gatherOVSFlows(ctx context.Context, config *rest.Config) ([]record.Record, []error) {
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, []error{err}
	}

	ovsPods, err := kubeClient.CoreV1().Pods(sdnNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=ovs",
	})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	records := make([]record.Record, 0, len(ovsPods.Items))
	for _, op := range ovsPods.Items {
		ovsFlow, stderr, err := ExecCmd(kubeClient.CoreV1(), config, op.Name, sdnNamespace, ovsCommand)
		if err != nil {
			if len(stderr) == 0 {
				stderr = []byte(err.Error())
			}
			klog.Warningf("Command \"%s\" failed in the %s pod: %s", ovsCommand, op.Name, stderr)
		}
		if len(ovsFlow) == 0 {
			continue
		}
		records = append(records, record.Record{
			Name: fmt.Sprintf("networking/ovs_flows/%s", op.Name),
			Item: ovsFlow,
		})
	}
	return records, nil
}
