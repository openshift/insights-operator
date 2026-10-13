// nolint: dupl
package clusterconfig

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

// GatherNodeNetworkState Collects cluster scope "nodenetworkstate.nmstate.io/v1beta1"
// resources
//
// ### API Reference
// - https://github.com/nmstate/kubernetes-nmstate/blob/main/api/v1beta1/nodenetworkstate_types.go
//
// ### Sample data
// - docs/insights-archive-sample/cluster-scoped-resources/nmstate.io/nodenetworkstates/etcd-quorum-guard.json
//
// ### Location in archive
// - `cluster-scoped-resources/nmstate.io/nodenetworkstates/{name}.json`
//
// ### Config ID
// `clusterconfig/nodenetworkstates`
//
// ### Released version
// - 4.18.0
//
// ### Backported versions
//
// ### Changes
func (g *Gatherer) GatherNodeNetworkState(ctx context.Context) ([]record.Record, []error) {
	dynCli, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}
	return gatherNodeNetworkState(ctx, dynCli)
}

func gatherNodeNetworkState(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	nodeNetStateList, err := dynamicClient.Resource(nodeNetStatesV1Beta1GVR).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	records := []record.Record{}
	errs := []error{}

	for i := range nodeNetStateList.Items {
		nodeNetworkState := nodeNetStateList.Items[i]
		err := anonymizeNodeNetworkState(nodeNetworkState.Object)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		records = append(records, record.Record{
			Name: fmt.Sprintf("cluster-scoped-resources/nmstate.io/nodenetworkstates/%s", nodeNetworkState.GetName()),
			Item: record.ResourceMarshaller{Resource: &nodeNetworkState},
		})
	}

	return records, errs
}

func anonymizeNodeNetworkState(nodeNetworkState map[string]interface{}) error {
	networInterfaces, err := utils.NestedSliceWrapper(nodeNetworkState, "status", "currentState", "interfaces")
	if err != nil {
		klog.Warning(err)
		return nil
	}
	anonymizedInterfaces := []interface{}{}
	for _, networInterface := range networInterfaces {
		networInterfaceMap, ok := networInterface.(map[string]interface{})
		if !ok {
			klog.Errorf("cannot cast the interface type: %v", err)
			continue
		}
		macAddress, err := utils.NestedStringWrapper(networInterfaceMap, "mac-address")
		if err != nil {
			// if there's no "mac-address" attribute, we still want to keep the interface
			anonymizedInterfaces = append(anonymizedInterfaces, networInterfaceMap)
			continue
		}
		err = unstructured.SetNestedField(networInterfaceMap, anonymize.String(macAddress), "mac-address")
		if err != nil {
			klog.Errorf("cannot anonymize the nodenetworkstate attribute: %v", err)
			continue
		}
		anonymizedInterfaces = append(anonymizedInterfaces, networInterfaceMap)
	}
	return unstructured.SetNestedSlice(nodeNetworkState, anonymizedInterfaces, "status", "currentState", "interfaces")
}
