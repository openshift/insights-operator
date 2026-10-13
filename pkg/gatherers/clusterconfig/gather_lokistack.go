package clusterconfig

// nolint: dupl

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
)

const lokiStackResourceLimit = 20

// GatherLokiStack Collects `lokistacks.loki.grafana.com` resources.
//
// The gatherer will collect up to 20 resources from `openshift-*` namespaces
// and it will report errors if it finds a `LokiStack` resource in a different namespace
// or if there are more than 20 `LokiStacks` in the `openshift-*` namespaces.
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/namespaces/openshift-logging/lokistack-sample.json
//
// ### Location in archive
// - `namespace/{namespace}/loki.grafana.com/lokistacks/{name}.json`
//
// ### Config ID
// `clusterconfig/lokistacks`
//
// ### Released version
// - 4.19.0
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
	loggingResourceList, err := dynamicClient.Resource(lokiStackResource).List(ctx, metav1.ListOptions{})

	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		klog.V(2).Infof("Unable to list %s resource due to: %s", lokiStackResource, err)
		return nil, []error{err}
	}

	var records []record.Record
	var errs []error
	otherNamespaceError := false
	tooManyResourcesError := false

	for index := range loggingResourceList.Items {
		item := loggingResourceList.Items[index]

		namespace := item.GetNamespace()
		if !strings.HasPrefix(namespace, "openshift-") {
			klog.Infof("LokiStack resource found in an unexpected namespace %s", namespace)
			if !otherNamespaceError {
				otherNamespaceError = true
				errs = append(errs, fmt.Errorf("found resource in an unexpected namespace"))
			}

			continue
		}

		if len(records) >= lokiStackResourceLimit {
			if !tooManyResourcesError {
				tooManyResourcesError = true
				errs = append(errs, fmt.Errorf(
					"found %d resources, limit (%d) reached",
					len(loggingResourceList.Items), lokiStackResourceLimit),
				)
			}
			continue
		}
		anonymizedRecord, err := fillLokiStackRecord(item)
		records = append(records, *anonymizedRecord)
		if err != nil {
			errs = append(errs, err)
		}
	}

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

		err = unstructured.SetNestedSlice(obj, streamSlice, "spec", "limits", "tenants", tenant, "retention", "streams")
		if err != nil {
			klog.V(2).Infof("Failed to set the anonymized slice for tenant %s", tenant)
			return err
		}
	}

	return nil
}
