package clusterconfig

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
)

type inferenceService struct {
	Namespace       string `json:"namespace"`
	Name            string `json:"name"`
	ModelFormatName string `json:"modelFormatName,omitempty"`
	ModelFormatVer  string `json:"modelFormatVersion,omitempty"`
	Runtime         string `json:"runtime,omitempty"`
	ProtocolVersion string `json:"protocolVersion,omitempty"`
}

// GatherInferenceServices Collects InferenceService resources from the KServe operator
// across all namespaces. Only selected non-sensitive fields are collected:
// model format name and version, runtime, and protocol version.
// The storageUri field is explicitly excluded as it may contain S3 bucket names or internal paths.
//
// ### API Reference
// - https://kserve.github.io/website/docs/reference/crd-api#inferenceservice
//
// ### Sample data
// - docs/insights-archive-sample/config/serving.kserve.io/inferenceservices.json
//
// ### Location in archive
// - `config/serving.kserve.io/inferenceservices.json`
//
// ### Config ID
// `clusterconfig/inference_services`
//
// ### Released version
// - 5.1
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherInferenceServices(ctx context.Context) ([]record.Record, []error) {
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherInferenceServices(ctx, dynamicClient)
}

func gatherInferenceServices(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	infereneceServiceList, err := dynamicClient.Resource(inferenceServiceGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	if len(infereneceServiceList.Items) == 0 {
		return nil, nil
	}

	inferenceServices := make([]inferenceService, 0, len(infereneceServiceList.Items))
	for i := range infereneceServiceList.Items {
		item := &infereneceServiceList.Items[i]
		svc := inferenceService{
			Namespace: item.GetNamespace(),
			Name:      item.GetName(),
		}
		if v, err := utils.NestedStringWrapper(item.Object, "spec", "predictor", "model", "modelFormat", "name"); err == nil {
			svc.ModelFormatName = v
		}
		if v, err := utils.NestedStringWrapper(item.Object, "spec", "predictor", "model", "modelFormat", "version"); err == nil {
			svc.ModelFormatVer = v
		}
		if v, err := utils.NestedStringWrapper(item.Object, "spec", "predictor", "model", "runtime"); err == nil {
			svc.Runtime = v
		}
		if v, err := utils.NestedStringWrapper(item.Object, "spec", "predictor", "model", "protocolVersion"); err == nil {
			svc.ProtocolVersion = v
		}
		inferenceServices = append(inferenceServices, svc)
	}

	return []record.Record{{
		Name: "config/serving.kserve.io/inferenceservices",
		Item: record.JSONMarshaller{Object: inferenceServices},
	}}, nil
}
