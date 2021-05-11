package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
)

var (
	sapMachineConfigNameList = map[string]struct{}{
		"75-worker-sap-data-intelligence": {},
		"75-master-sap-data-intelligence": {},
	}
)

// GatherSAPMachineConfigs collects a subset of MachineConfigs related to SDI by applying a set of filtering rules.
//
// Gathered MachineConfigs at the time of implementation of the gatherer:
// * `75-worker-sap-data-intelligence`
// * `75-master-sap-data-intelligence`
// * `99-sdi-generated-containerruntime`
//
// Response see https://docs.openshift.com/container-platform/4.7/rest_api/machine_apis/machineconfig-machineconfiguration-openshift-io-v1.html
//
// * Location in archive: config/machineconfigs/<name>.json
// * Id in config: sap_machine_configs
// * Since versions:
//   * 4.9+
func GatherSAPMachineConfigs(g *Gatherer, c chan<- gatherResult) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{errors: []error{err}}
		return
	}

	records, errs := gatherSAPMachineConfigs(g.ctx, gatherDynamicClient)
	c <- gatherResult{records: records, errors: errs}
}

func isSAPMachineConfig(mc unstructured.Unstructured) bool {
	if _, exists := sapMachineConfigNameList[mc.GetName()]; exists {
		return true
	}

	for labelName, labelValue := range mc.GetLabels() {
		if labelName == "workload" && labelValue == "sap-data-intelligence" {
			return true
		}
	}

	for _, ownerRef := range mc.GetOwnerReferences() {
		if ownerRef.Kind == "ContainerRuntimeConfig" && ownerRef.Name == "sdi-pids-limit" {
			return true
		}
	}

	return false
}

func gatherSAPMachineConfigs(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	mcList, err := dynamicClient.Resource(machineConfigGroupVersionResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	records := []record.Record{}
	for _, mc := range mcList.Items {
		if isSAPMachineConfig(mc) {
			records = append(records, record.Record{
				Name: fmt.Sprintf("config/machineconfigs/%s", mc.GetName()),
				Item: record.JSONMarshaller{Object: mc.Object},
			})
		}
	}

	return records, nil
}
