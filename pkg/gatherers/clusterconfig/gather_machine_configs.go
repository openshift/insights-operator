package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	mcfgclientset "github.com/openshift/client-go/machineconfiguration/clientset/versioned"
	"github.com/openshift/insights-operator/pkg/record"
)

// UnusedMachineConfigsCount represents the count of unused MachineConfig in the cluster
type UnusedMachineConfigsCount struct {
	UnusedCount uint `json:"unused_machineconfigs_count"`
}

// GatherMachineConfigs Collects definitions of in-use 'MachineConfigs'. MachineConfig is used when it's referenced in
// a MachineConfigPool or in Node `machineconfiguration.openshift.io/desiredConfig` and `machineconfiguration.openshift.io/currentConfig`
// annotations
// Following data is intentionally removed from the definitions:
// - `spec.config.storage.files`
// - `spec.config.passwd.users`
//
// ### API Reference
// - https://docs.openshift.com/container-platform/4.7/rest_api/machine_apis/machineconfig-machineconfiguration-openshift-io-v1.html
//
// ### Sample data
// - docs/insights-archive-sample/aggregated/unused_machine_configs_count.json
// - docs/insights-archive-sample/config/machineconfigs/75-worker-sap-data-intelligence.json
//
// ### Location in archive
// - `aggregated/unused_machine_configs_count.json`
// - `config/machineconfigs/{resource}.json`
//
// ### Config ID
// `clusterconfig/machine_configs`
//
// ### Released version
// - 4.9.0
//
// ### Backported versions
// - 4.8.5
//
// ### Changes
// - gathers only in-use MachineConfigs since 4.18+
func (g *Gatherer) GatherMachineConfigs(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}
	var errs []error
	inUseMachineConfigs, err := getInUseMachineConfigs(ctx, g.gatherKubeConfig)
	if err != nil {
		errs = append(errs, err)
	}
	records, gatherErrs := gatherMachineConfigs(ctx, gatherDynamicClient, inUseMachineConfigs)
	errs = append(errs, gatherErrs...)
	return records, errs
}

func gatherAllMachineConfigs(ctx context.Context, dynamicClient dynamic.Interface) ([]unstructured.Unstructured, error) {
	mcList, err := dynamicClient.Resource(machineConfigGroupVersionResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return mcList.Items, nil
}

func gatherMachineConfigs(ctx context.Context, dynamicClient dynamic.Interface,
	inUseMachineConfigs sets.Set[string]) ([]record.Record, []error) {
	const unusedCountFilename string = "aggregated/unused_machine_configs_count"
	count := UnusedMachineConfigsCount{UnusedCount: 0}

	items, err := gatherAllMachineConfigs(ctx, dynamicClient)
	if err != nil {
		return nil, []error{err}
	}

	records := []record.Record{}
	var errs []error
	for i := range items {
		mc := items[i]
		// skip machine configs which are not in use
		if len(inUseMachineConfigs) != 0 && !inUseMachineConfigs.Has(mc.GetName()) {
			count.UnusedCount++
			continue
		}
		// remove the sensitive content by overwriting the values
		err := unstructured.SetNestedField(mc.Object, nil, "spec", "config", "storage", "files")
		if err != nil {
			klog.Errorf("unable to set nested field: %v", err)
			errs = append(errs, err)
		}
		err = unstructured.SetNestedField(mc.Object, nil, "spec", "config", "passwd", "users")
		if err != nil {
			klog.Errorf("unable to set nested field: %v", err)
			errs = append(errs, err)
		}
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/machineconfigs/%s", mc.GetName()),
			Item: record.ResourceMarshaller{Resource: &mc},
		})
	}
	if len(errs) > 0 {
		return records, errs
	}

	records = append(records, record.Record{
		Name: unusedCountFilename,
		Item: record.JSONMarshaller{Object: count},
	})
	return records, nil
}

// GetInUseMachineConfigs filters in-use MachineConfig resources and returns set of their names.
func getInUseMachineConfigs(ctx context.Context, clientConfig *rest.Config) (sets.Set[string], error) {
	// Create a set to store in-use configs
	inuseConfigs := sets.New[string]()

	machineConfigClient, err := mcfgclientset.NewForConfig(clientConfig)
	if err != nil {
		return nil, err
	}

	poolList, err := machineConfigClient.MachineconfigurationV1().MachineConfigPools().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting MachineConfigPools failed: %w", err)
	}

	for i := range poolList.Items {
		pool := poolList.Items[i]
		// Get the rendered config name from the status section
		inuseConfigs.Insert(pool.Status.Configuration.Name)
		inuseConfigs.Insert(pool.Spec.Configuration.Name)
	}

	kubeClient, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return nil, err
	}
	nodeList, err := kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i := range nodeList.Items {
		node := nodeList.Items[i]
		current, ok := node.Annotations["machineconfiguration.openshift.io/currentConfig"]
		if ok {
			inuseConfigs.Insert(current)
		}
		desired, ok := node.Annotations["machineconfiguration.openshift.io/desiredConfig"]
		if ok {
			inuseConfigs.Insert(desired)
		}
	}

	return inuseConfigs, nil
}
