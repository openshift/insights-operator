package clusterconfig

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
)

type externalModel struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	Provider    string `json:"provider,omitempty"`
	TargetModel string `json:"targetModel,omitempty"`
}

// GatherExternalModels Collects ExternalModel resources from the MaaS (Model as a Service) operator
// across all namespaces. Only the provider and target model are collected.
//
// ### API Reference
// - https://github.com/opendatahub-io/models-as-a-service/blob/main/maas-controller/api/maas/v1alpha1/externalmodel_types.go
//
// ### Sample data
// - docs/insights-archive-sample/config/externalmodels.json
//
// ### Location in archive
// - `config/externalmodels.json`
//
// ### Config ID
// `clusterconfig/external_models`
//
// ### Released version
// - 5.1
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherExternalModels(ctx context.Context) ([]record.Record, []error) {
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherExternalModels(ctx, dynamicClient)
}

func gatherExternalModels(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	externalModelList, err := dynamicClient.Resource(externalModelGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	if len(externalModelList.Items) == 0 {
		return nil, nil
	}

	externalModels := make([]externalModel, 0, len(externalModelList.Items))
	for i := range externalModelList.Items {
		item := &externalModelList.Items[i]
		em := externalModel{
			Namespace: item.GetNamespace(),
			Name:      item.GetName(),
		}
		if v, err := utils.NestedStringWrapper(item.Object, "spec", "provider"); err == nil {
			em.Provider = v
		}
		if v, err := utils.NestedStringWrapper(item.Object, "spec", "targetModel"); err == nil {
			em.TargetModel = v
		}
		externalModels = append(externalModels, em)
	}

	return []record.Record{{
		Name: "config/externalmodels",
		Item: record.JSONMarshaller{Object: externalModels},
	}}, nil
}
