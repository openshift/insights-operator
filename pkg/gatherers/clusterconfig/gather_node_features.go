package clusterconfig

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"

	"github.com/openshift/insights-operator/pkg/record"
)

// List of attribute fields that is used to filter the NodeFeatureSpec
// fields that should be included in the gathered data
var allowedAttributesFields = []string{
	"cpu.topology",
	"system.dmiid",
}

// GatherNodeFeatures Collects `nodefeatures.nfd.k8s-sigs.io` custom resources
// from the openshift-nfd namespace.
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/namespaces/openshift-nfd/customresources/{name}.json
//
// ### Location in archive
// - `namespaces/openshift-nfd/customresources/{name}.json`
//
// ### Config ID
// `clusterconfig/node_features`
//
// ### Released version
// - 4.21.0
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherNodeFeatures(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherNodeFeatures(ctx, gatherDynamicClient)
}

func gatherNodeFeatures(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	records, err := getNodeFeaturesData(ctx, dynamicClient)
	if err != nil {
		klog.Errorf("GatherNodeFeatures: Failed to get NodeFeatures data: %v", err)
		return nil, []error{err}
	}

	return records, nil
}

func filterNodeFeatureSpec(nodeFeature *nfdv1alpha1.NodeFeatureSpec) *nfdv1alpha1.NodeFeatureSpec {
	filteredNodeFeatureSpec := nfdv1alpha1.NodeFeatureSpec{
		Features: nfdv1alpha1.Features{
			Attributes: make(map[string]nfdv1alpha1.AttributeFeatureSet),
		},
	}

	// Filter attribute keys
	for _, key := range allowedAttributesFields {
		if value, exists := nodeFeature.Features.Attributes[key]; exists {
			filteredNodeFeatureSpec.Features.Attributes[key] = value
		}
	}

	return &filteredNodeFeatureSpec
}

func getNodeFeaturesData(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, error) {
	nodeFeaturesList, err := dynamicClient.Resource(nodeFeatureResource).Namespace("openshift-nfd").List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		klog.Errorf("GatherNodeFeatures: NodeFeatures resource not found in openshift-nfd namespace (may not be installed)")
		return nil, nil
	}
	if err != nil {
		klog.Errorf("GatherNodeFeatures: Failed to list NodeFeatures: %v", err)
		return nil, err
	}

	var records []record.Record

	for _, nodeFeature := range nodeFeaturesList.Items {
		// Marshal the unstructured object to JSON
		data, err := json.Marshal(nodeFeature.Object)
		if err != nil {
			klog.Errorf("GatherNodeFeatures: Failed to marshal NodeFeature: %v", err)
			continue
		}

		// Unmarshal into our NodeFeature struct
		var typedNodeFeature nfdv1alpha1.NodeFeature
		if err := json.Unmarshal(data, &typedNodeFeature); err != nil {
			klog.Errorf("GatherNodeFeatures: Failed to unmarshal NodeFeature: %v", err)
			continue
		}

		// Filter only allowed spec fields
		filteredNodeFeatureSpec := filterNodeFeatureSpec(&typedNodeFeature.Spec)

		recordName := fmt.Sprintf("namespaces/openshift-nfd/customresources/%s",
			typedNodeFeature.Name,
		)

		records = append(records, record.Record{
			Name: recordName,
			Item: record.JSONMarshaller{Object: *filteredNodeFeatureSpec},
		})
	}

	return records, nil
}
