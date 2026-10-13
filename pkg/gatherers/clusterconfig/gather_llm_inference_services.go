package clusterconfig

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
)

// llmInferenceServiceRecordsLimit caps how many LLMInferenceService resources are
// gathered. RHOAI/llm-d creates a namespaced LLMInferenceService per deployed model,
// so the count scales with the number of model deployments.
const llmInferenceServiceRecordsLimit = 100

// llmInferenceServiceInlineConfigMaxBytes caps the serialized size of the inline
// scheduler config so a single resource can't bloat the archive.
const llmInferenceServiceInlineConfigMaxBytes = 10 * 1024

type llmInferenceService struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`

	Model *llmModel `json:"model,omitempty"`

	Replicas *int64 `json:"replicas,omitempty"`

	PrefillPresent  bool   `json:"prefillPresent,omitempty"`
	PrefillReplicas *int64 `json:"prefillReplicas,omitempty"`

	Parallelism *llmParallelism `json:"parallelism,omitempty"`

	RouterSchedulerPresent      bool                   `json:"routerSchedulerPresent,omitempty"`
	RouterSchedulerConfigInline map[string]interface{} `json:"routerSchedulerConfigInline,omitempty"`

	WorkloadsSchedulerPresent bool `json:"workloadsSchedulerPresent,omitempty"`

	Scaling *llmScaling `json:"scaling,omitempty"`

	TracingPresent bool `json:"tracingPresent,omitempty"`

	Containers []llmContainer `json:"containers,omitempty"`
}

type llmModel struct {
	Name             string   `json:"name,omitempty"`
	LoraAdapterNames []string `json:"loraAdapterNames,omitempty"`
	LoraMaxAdapters  *int64   `json:"loraMaxAdapters,omitempty"`
}

type llmParallelism struct {
	Tensor   int64 `json:"tensor,omitempty"`
	Pipeline int64 `json:"pipeline,omitempty"`
	Data     int64 `json:"data,omitempty"`
	Expert   *bool `json:"expert,omitempty"`
}

type llmScaling struct {
	MinReplicas *int64 `json:"minReplicas,omitempty"`
	MaxReplicas *int64 `json:"maxReplicas,omitempty"`
}

type llmContainer struct {
	Image     string            `json:"image,omitempty"`
	Resources map[string]string `json:"resourceLimits,omitempty"`
}

// GatherLLMInferenceServices Collects LLMInferenceService resources from the KServe
// operator (llm-d) across all namespaces. Only selected non-sensitive fields are
// collected: model name and LoRA adapter names/maxAdapters, replica counts,
// parallelism settings, presence of the prefill/router-scheduler/tracing/
// workloads-scheduler blocks, the inline scheduler config (with sensitive keys
// redacted and size capped), scaling bounds, and container image/resource limits.
//
// The lora.adapters[].uri field is explicitly excluded as it may contain storage
// paths, and containers[].env is never collected as it commonly holds tokens and
// API keys.
//
// ### API Reference
// - https://kserve.github.io/website/docs/reference/crd-api#llminferenceservice
//
// ### Sample data
// - docs/insights-archive-sample/config/serving.kserve.io/llminferenceservices.json
//
// ### Location in archive
// - `config/serving.kserve.io/llminferenceservices.json`
//
// ### Config ID
// `clusterconfig/llm_inference_services`
//
// ### Released version
// - 5.1
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherLLMInferenceServices(ctx context.Context) ([]record.Record, []error) {
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherLLMInferenceServices(ctx, dynamicClient)
}

func gatherLLMInferenceServices(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	list, err := dynamicClient.Resource(llmInferenceServiceGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	if len(list.Items) == 0 {
		return nil, nil
	}

	services, errs := extractLLMInferenceServices(list.Items, llmInferenceServiceGVR)

	return []record.Record{{
		Name: "config/serving.kserve.io/llminferenceservices",
		Item: record.JSONMarshaller{Object: services},
	}}, errs
}

// extractLLMInferenceServices converts the unstructured resources into the gathered
// form, capping the result at llmInferenceServiceRecordsLimit.
func extractLLMInferenceServices(items []unstructured.Unstructured,
	gvr schema.GroupVersionResource,
) ([]llmInferenceService, []error) {
	var errs []error

	limit := len(items)
	if limit > llmInferenceServiceRecordsLimit {
		limit = llmInferenceServiceRecordsLimit
		errs = append(errs, fmt.Errorf("found %d %s resources, limiting to %d",
			len(items), gvr.GroupResource(), llmInferenceServiceRecordsLimit))
	}

	services := make([]llmInferenceService, 0, limit)
	for i := range items[:limit] {
		svc, svcErrs := extractLLMInferenceService(&items[i])
		errs = append(errs, svcErrs...)
		services = append(services, svc)
	}

	return services, errs
}

func extractLLMInferenceService(item *unstructured.Unstructured) (llmInferenceService, []error) {
	var errs []error

	svc := llmInferenceService{
		Namespace: item.GetNamespace(),
		Name:      item.GetName(),
	}

	svc.Model = extractLLMModel(item.Object)

	if v, err := utils.NestedInt64Wrapper(item.Object, "spec", "replicas"); err == nil {
		svc.Replicas = &v
	}

	if _, found, _ := unstructured.NestedMap(item.Object, "spec", "prefill"); found {
		svc.PrefillPresent = true
	}
	if v, err := utils.NestedInt64Wrapper(item.Object, "spec", "prefill", "replicas"); err == nil {
		svc.PrefillReplicas = &v
	}

	svc.Parallelism = extractLLMParallelism(item.Object)

	if _, found, _ := unstructured.NestedMap(item.Object, "spec", "router", "scheduler"); found {
		svc.RouterSchedulerPresent = true
	}
	if inline, found, err := unstructured.NestedMap(item.Object, "spec", "router", "scheduler", "config", "inline"); found && err == nil {
		cfg, cfgErr := sanitizeInlineSchedulerConfig(inline)
		svc.RouterSchedulerConfigInline = cfg
		if cfgErr != nil {
			errs = append(errs, cfgErr)
		}
	}

	if _, found, _ := unstructured.NestedMap(item.Object, "status", "workloads", "scheduler"); found {
		svc.WorkloadsSchedulerPresent = true
	}

	svc.Scaling = extractLLMScaling(item.Object)

	if _, found, _ := unstructured.NestedMap(item.Object, "spec", "tracing"); found {
		svc.TracingPresent = true
	}

	if containers, err := utils.NestedSliceWrapper(item.Object, "spec", "template", "containers"); err == nil {
		svc.Containers = extractLLMContainers(containers)
	}

	return svc, errs
}

func extractLLMModel(obj map[string]interface{}) *llmModel {
	name, nameErr := utils.NestedStringWrapper(obj, "spec", "model", "name")
	adapters, adaptersErr := utils.NestedSliceWrapper(obj, "spec", "model", "lora", "adapters")
	maxAdapters, maxAdaptersErr := utils.NestedInt64Wrapper(obj, "spec", "model", "lora", "maxAdapters")

	if nameErr != nil && adaptersErr != nil && maxAdaptersErr != nil {
		return nil
	}

	model := &llmModel{}
	if nameErr == nil {
		model.Name = name
	}
	if adaptersErr == nil {
		model.LoraAdapterNames = extractLoraAdapterNames(adapters)
	}
	if maxAdaptersErr == nil {
		model.LoraMaxAdapters = &maxAdapters
	}

	return model
}

// extractLoraAdapterNames reads only the adapter name. The adapter uri is
// deliberately never read as it may reference an object storage path.
func extractLoraAdapterNames(adapters []interface{}) []string {
	var names []string
	for _, a := range adapters {
		adapterMap, ok := a.(map[string]interface{})
		if !ok {
			continue
		}
		if name, found, err := unstructured.NestedString(adapterMap, "name"); found && err == nil {
			names = append(names, name)
		}
	}
	return names
}

func extractLLMParallelism(obj map[string]interface{}) *llmParallelism {
	tensor, tensorErr := utils.NestedInt64Wrapper(obj, "spec", "parallelism", "tensor")
	pipeline, pipelineErr := utils.NestedInt64Wrapper(obj, "spec", "parallelism", "pipeline")
	data, dataErr := utils.NestedInt64Wrapper(obj, "spec", "parallelism", "data")
	expert, expertFound, expertErr := unstructured.NestedBool(obj, "spec", "parallelism", "expert")

	if tensorErr != nil && pipelineErr != nil && dataErr != nil && (!expertFound || expertErr != nil) {
		return nil
	}

	parallelism := &llmParallelism{}
	if tensorErr == nil {
		parallelism.Tensor = tensor
	}
	if pipelineErr == nil {
		parallelism.Pipeline = pipeline
	}
	if dataErr == nil {
		parallelism.Data = data
	}
	if expertFound && expertErr == nil {
		parallelism.Expert = &expert
	}

	return parallelism
}

func extractLLMScaling(obj map[string]interface{}) *llmScaling {
	minReplicas, minErr := utils.NestedInt64Wrapper(obj, "spec", "scaling", "minReplicas")
	maxReplicas, maxErr := utils.NestedInt64Wrapper(obj, "spec", "scaling", "maxReplicas")

	if minErr != nil && maxErr != nil {
		return nil
	}

	scaling := &llmScaling{}
	if minErr == nil {
		scaling.MinReplicas = &minReplicas
	}
	if maxErr == nil {
		scaling.MaxReplicas = &maxReplicas
	}

	return scaling
}

// extractLLMContainers reads the container image and resource limits.
// The env field is deliberately never read - it commonly holds tokens and API keys.
func extractLLMContainers(containers []interface{}) []llmContainer {
	var result []llmContainer
	for _, c := range containers {
		containerMap, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		var container llmContainer
		container.Image, _, _ = unstructured.NestedString(containerMap, "image")
		if limits, found, err := unstructured.NestedStringMap(containerMap, "resources", "limits"); found && err == nil {
			container.Resources = limits
		}
		result = append(result, container)
	}
	return result
}

// sanitizeInlineSchedulerConfig redacts sensitive keys from the inline scheduler
// config (an arbitrary, admin-authored EndpointPickerConfig blob) and drops it
// entirely if its serialized size exceeds llmInferenceServiceInlineConfigMaxBytes.
func sanitizeInlineSchedulerConfig(cfg map[string]interface{}) (map[string]interface{}, error) {
	anonymizeMap(cfg, false)

	marshaled, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal router.scheduler.config.inline: %w", err)
	}
	if len(marshaled) > llmInferenceServiceInlineConfigMaxBytes {
		return nil, fmt.Errorf("router.scheduler.config.inline exceeds %d bytes, omitting",
			llmInferenceServiceInlineConfigMaxBytes)
	}

	return cfg, nil
}
