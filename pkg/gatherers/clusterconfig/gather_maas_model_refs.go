package clusterconfig

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
)

type maasModelRef struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Kind      string `json:"kind,omitempty"`
}

// GatherMaasModelRefs Collects MaasModelRef resources from the MaaS (Model as a Service) operator
// across all namespaces. For each resource, the namespace, resource name, and referenced model kind
// are collected.
//
// ### API Reference
// - https://github.com/opendatahub-io/models-as-a-service/blob/main/maas-controller/api/maas/v1alpha1/maasmodelref_types.go
//
// ### Sample data
// - docs/insights-archive-sample/config/maasmodelrefs.json
//
// ### Location in archive
// - `config/maasmodelrefs.json`
//
// ### Config ID
// `clusterconfig/maas_model_refs`
//
// ### Released version
// - 5.1
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherMaasModelRefs(ctx context.Context) ([]record.Record, []error) {
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherMaasModelRefs(ctx, dynamicClient)
}

func gatherMaasModelRefs(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	maasModelRefList, err := dynamicClient.Resource(maasModelRefGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	if len(maasModelRefList.Items) == 0 {
		return nil, nil
	}

	maasModelRefs := make([]maasModelRef, 0, len(maasModelRefList.Items))
	for i := range maasModelRefList.Items {
		item := &maasModelRefList.Items[i]
		ref := maasModelRef{
			Namespace: item.GetNamespace(),
			Name:      item.GetName(),
		}
		if v, err := utils.NestedStringWrapper(item.Object, "spec", "modelRef", "kind"); err == nil {
			ref.Kind = v
		}
		maasModelRefs = append(maasModelRefs, ref)
	}

	return []record.Record{{
		Name: "config/maasmodelrefs",
		Item: record.JSONMarshaller{Object: maasModelRefs},
	}}, nil
}
