package clusterconfig

// nolint: dupl

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
)

const LokiStackResourceLimit = 20

// GatherOpenshiftLogging Collects `clusterlogging.logging.openshift.io` resources.
//
// ### API Reference
// - https://github.com/openshift/cluster-logging-operator/blob/master/pkg/apis/logging/v1/clusterlogging_types.go
//
// ### Sample data
// - docs/insights-archive-sample/config/logging/openshift-logging/instance.json
//
// ### Location in archive
// - `namespace/openshift-logging/group/resource/{name}.json`
//
// ### Config ID
// `clusterconfig/openshift_logging`
//
// ### Released version
// - 4.18.0
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherLokiStack(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherLokiStack(ctx, gatherDynamicClient)
}

func gatherLokiStack(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	klog.V(2).Info("Start LokiStack gathering")
	loggingResourceList, err := dynamicClient.Resource(lokiStackResource).
		Namespace("openshift-logging").
		List(ctx, metav1.ListOptions{Limit: LokiStackResourceLimit})

	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		klog.V(2).Infof("Unable to list %s resource due to: %s", lokiStackResource, err)
		return nil, []error{err}
	}

	var records []record.Record
	var errs []error

	numItems := len(loggingResourceList.Items)
	if numItems > 1 {
		errs = append(errs, fmt.Errorf("Found more resources than expected (expected 1)"))
	}

	if loggingResourceList.GetContinue() != "" {
		errs = append(errs, fmt.Errorf("Found more than %d resources", LokiStackResourceLimit))
	}

	for index, _ := range loggingResourceList.Items {
		item := loggingResourceList.Items[index]
		record, err := fillLokiStackRecord(item)
		records = append(records, *record)
		if err != nil {
			errs = append(errs, err)
		}
	}

	// numItems == 0
	return records, errs
}

func fillLokiStackRecord(item unstructured.Unstructured) (*record.Record, error) {
	if err := removeLimitsTenant(item.Object); err != nil {
		return nil, err
	}

	return &record.Record{
		Name: fmt.Sprintf(
			"namespace/%s/%s/%s/%s",
			item.GetNamespace(),
			lokiStackResource.Group,
			lokiStackResource.Resource,
			item.GetName()),
		Item: record.ResourceMarshaller{Resource: &item},
	}, nil
}

// removeLimitsTenant tries to get an array of sensitive fields defined in the LokiStack
// and anonymize potentially sensitive data - e.g. url, credentials
func removeLimitsTenant(obj map[string]interface{}) error {
	// unstructured.RemoveNestedField(obj, "spec", "limits", "tenants", "application", "streams", "selector")
	// unstructured.RemoveNestedField(obj, "spec", "limits", "tenants", "audit", "streams", "selector")
	// unstructured.RemoveNestedField(obj, "spec", "limits", "tenants", "infrastructure", "streams", "selector")

	for _, tenant := range []string{"application", "infrastructure", "audit"} {
		klog.V(2).Infof("Anonymizing %s tenant", tenant)
		streamSlice, ok, err := unstructured.NestedSlice(obj, "spec", "limits", "tenants", tenant, "retention", "streams")
		if err != nil {
			klog.V(2).Infof("Bad structure for the gathered file: %v", err)
			return err
		} else if !ok {
			// tenant not found
			continue
		}

		for _, stream := range streamSlice {
			streamMap, ok := stream.(map[string]interface{})
			if !ok {
				continue
			}
			unstructured.RemoveNestedField(streamMap, "selector")
		}

		unstructured.SetNestedSlice(obj, streamSlice, "spec", "limits", "tenants", tenant, "retention", "streams")
	}

	return nil
}
