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

const clusterLogForwarderResourceLimit = 20

// GatherClusterLogForwarder Collects `clusterlogforwarders.observability.openshift.io` resources.
//
// The gatherer will collect up to 20 resources from `openshift-*` namespaces
// and it will report errors if it finds a `ClusterLogForwarder` resource in a different namespace
// or if there are more than 20 `ClusterLogForwarders` in the `openshift-*` namespaces.
//
// Sensitive fields like output URLs, authentication credentials, and TLS configuration
// are removed from the collected data.
//
// ### API Reference
// - https://github.com/openshift/cluster-logging-operator/blob/master/api/observability/v1/clusterlogforwarder_types.go
//
// ### Sample data
// - docs/insights-archive-sample/namespaces/openshift-logging/clusterlogforwarder-sample.json
//
// ### Location in archive
// - `namespace/{namespace}/observability.openshift.io/clusterlogforwarders/{name}.json`
//
// ### Config ID
// `clusterconfig/clusterlogforwarders`
//
// ### Released version
// - 5.1.0
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherClusterLogForwarder(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherClusterLogForwarder(ctx, gatherDynamicClient)
}

func gatherClusterLogForwarder(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	klog.V(2).Info("Start ClusterLogForwarder gathering")
	clfResourceList, err := dynamicClient.Resource(clusterLogForwarderResource).List(ctx, metav1.ListOptions{})

	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		klog.V(2).Infof("Unable to list %s resource due to: %s", clusterLogForwarderResource, err)
		return nil, []error{err}
	}

	var records []record.Record
	var errs []error
	otherNamespaceError := false
	tooManyResourcesError := false

	for index := range clfResourceList.Items {
		item := clfResourceList.Items[index]

		namespace := item.GetNamespace()
		if !strings.HasPrefix(namespace, "openshift-") {
			klog.Infof("ClusterLogForwarder resource found in an unexpected namespace %s", namespace)
			if !otherNamespaceError {
				otherNamespaceError = true
				errs = append(errs, fmt.Errorf("found resource in an unexpected namespace"))
			}

			continue
		}

		if len(records) >= clusterLogForwarderResourceLimit {
			if !tooManyResourcesError {
				tooManyResourcesError = true
				errs = append(errs, fmt.Errorf(
					"found %d resources, limit (%d) reached",
					len(clfResourceList.Items), clusterLogForwarderResourceLimit),
				)
			}
			continue
		}
		anonymizedRecord, err := fillClusterLogForwarderRecord(item)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		records = append(records, *anonymizedRecord)
	}

	return records, errs
}

func fillClusterLogForwarderRecord(item unstructured.Unstructured) (*record.Record, error) {
	annotations := item.GetAnnotations()
	delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
	item.SetAnnotations(annotations)

	if err := anonymizeClusterLogForwarderOutputs(item.Object); err != nil {
		return nil, err
	}

	return &record.Record{
		Name: fmt.Sprintf(
			"namespace/%s/%s/%s/%s",
			item.GetNamespace(),
			clusterLogForwarderResource.Group,
			clusterLogForwarderResource.Resource,
			item.GetName()),
		Item: record.ResourceMarshaller{Resource: &item},
	}, nil
}

// anonymizeClusterLogForwarderOutputs removes sensitive fields from CLF outputs:
// URLs, authentication, TLS configuration, headers, and broker endpoints.
func anonymizeClusterLogForwarderOutputs(obj map[string]interface{}) error {
	outputsSlice, ok, err := unstructured.NestedSlice(obj, "spec", "outputs")
	if err != nil {
		klog.V(2).Infof("Bad structure for the gathered file: %v", err)
		return err
	} else if !ok {
		return nil
	}

	for _, output := range outputsSlice {
		outputMap, ok := output.(map[string]interface{})
		if !ok {
			continue
		}

		unstructured.RemoveNestedField(outputMap, "tls")
		unstructured.RemoveNestedField(outputMap, "authentication")

		for _, outputType := range []string{
			"azureMonitor", "cloudwatch", "elasticsearch", "googleCloudLogging",
			"http", "kafka", "loki", "lokiStack", "otlp", "splunk", "syslog",
		} {
			outputTypeMap, exists, err := unstructured.NestedMap(outputMap, outputType)
			if err != nil {
				klog.V(2).Infof("Bad structure for output type %s: %v", outputType, err)
				return err
			}
			if !exists {
				continue
			}
			unstructured.RemoveNestedField(outputTypeMap, "url")
			unstructured.RemoveNestedField(outputTypeMap, "authentication")
			unstructured.RemoveNestedField(outputTypeMap, "headers")
			unstructured.RemoveNestedField(outputTypeMap, "proxyURL")
			unstructured.RemoveNestedField(outputTypeMap, "brokers")
			if err := unstructured.SetNestedMap(outputMap, outputTypeMap, outputType); err != nil {
				klog.V(2).Infof("Failed to set anonymized output type %s: %v", outputType, err)
			}
		}
	}

	return unstructured.SetNestedSlice(obj, outputsSlice, "spec", "outputs")
}
