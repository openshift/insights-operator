package conditional

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/record"
)

// BuildGatherCRDefinition collects custom resource definitions based on the GVR parameters provided
// from the namespaces that are firing one of the configured alerts.
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/conditional/namespaces/openshift-storage/crd/ceph.rook.io/v1/cephclusters/ocs-storagecluster-cephcluster.json
//
// ### Location in archive
// - `conditional/namespaces/{namespace}/crd/{group}/{version}/{resource}/{name}.json`
//
// ### Config ID
// `conditional/cr_definition`
//
// ### Released version
// - 4.23
//
// ### Backported versions
// - TBD
//
// ### Changes
// None
func (g *Gatherer) BuildGatherCRDefinition(paramsInterface interface{}) (gatherers.GatheringClosure, error) {
	params, ok := paramsInterface.(GatherCRDefinitionParams)
	if !ok {
		return gatherers.GatheringClosure{}, fmt.Errorf(
			"unexpected type in paramsInterface, expected %T, got %T",
			GatherCRDefinitionParams{},
			paramsInterface)
	}

	return gatherers.GatheringClosure{
		Run: func(ctx context.Context) ([]record.Record, []error) {
			dynamicClient, err := dynamic.NewForConfig(g.gatherProtoKubeConfig)
			if err != nil {
				return nil, []error{err}
			}
			return g.gatherCRDefinition(ctx, params, dynamicClient)
		},
	}, nil
}

func (g *Gatherer) gatherCRDefinition(
	ctx context.Context,
	params GatherCRDefinitionParams,
	dynamicClient dynamic.Interface,
) ([]record.Record, []error) {
	alertInstances, ok := g.firingAlerts[params.AlertName]
	if !ok {
		err := fmt.Errorf("conditional gather triggered, but specified alert %q is not firing", params.AlertName)
		return nil, []error{err}
	}

	const logMissingLabel = "%s at alertName: %s"

	var errs []error
	var records []record.Record

	for _, alertLabels := range alertInstances {
		namespace, err := getAlertPodNamespace(alertLabels)
		if err != nil {
			klog.Warningf(logMissingLabel, err.Error(), params.AlertName)
			errs = append(errs, err)
			continue
		}

		gvr := schema.GroupVersionResource{
			Group:    params.Group,
			Version:  params.Version,
			Resource: params.Resource,
		}

		crList, err := dynamicClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			klog.Warningf("CR not found in %s namespace (GVR: %v): %v", namespace, gvr, err)
			errs = append(errs, err)
			continue
		}

		for i := range crList.Items {
			item := &crList.Items[i]
			records = append(records, record.Record{
				Name: fmt.Sprintf(
					"%s/namespaces/%s/crd/%s/%s/%s/%s",
					g.GetName(),
					namespace,
					params.Group,
					params.Version,
					params.Resource,
					item.GetName()),
				Item: record.JSONMarshaller{Object: item.Object},
			})
		}
	}

	return records, errs
}
