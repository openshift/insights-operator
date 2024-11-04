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
	"github.com/openshift/insights-operator/pkg/utils"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
)

// GatherOpenshiftLogging Collects `clusterlogging.logging.openshift.io` resources.
//
// ### API Reference
// - https://github.com/openshift/cluster-logging-operator/blob/master/pkg/apis/logging/v1/clusterlogging_types.go
//
// ### Sample data
// - docs/insights-archive-sample/config/logging/openshift-logging/instance.json
//
// ### Location in archive
// - `config/logging/{namespace}/{name}.json`
//
// ### Config ID
// `clusterconfig/openshift_logging`
//
// ### Released version
// - 4.9.0
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
	loggingResourceList, err := dynamicClient.Resource(lokiStackResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		klog.V(2).Infof("Unable to list %s resource due to: %s", lokiStackResource, err)
		return nil, []error{err}
	}

	var records []record.Record
	for i := range loggingResourceList.Items {
		item := loggingResourceList.Items[i]
		err = anonymizeLokiStackNamespace(item.Object)
		if err != nil {
			// not storing resource with non-anonymized data
			continue
		}
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/lokistack/%s/%s", item.GetUID(), item.GetName()),
			Item: record.ResourceMarshaller{Resource: &item},
		})
	}
	return records, nil
}

// anonymizeLokiStackNamespace tries to get an array of sensitive fields defined in the LokiStack
// and anonymize potentially sensitive data - e.g. url, credentials
func anonymizeLokiStackNamespace(obj map[string]interface{}) error {
	namespace, err := utils.NestedStringWrapper(obj, "metadata", "namespace")
	if err != nil {
		// namespace not found, weird, but ok. Return silently
		klog.V(2).Info("Resource doesn't have namespace")
		return nil
	}

	if strings.HasPrefix(namespace, "openshift-") {
		// These namespaces don't need anonymization
		return nil
	}

	err = unstructured.SetNestedField(obj, anonymize.String(namespace), "metadata", "namespace")
	if err != nil {
		klog.V(2).ErrorS(err, "Failed to anonymize the namespace")
	}
	return err
}
