package clusterconfig

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
)

// servingRuntimeRecordsLimit caps how many runtimes are gathered per resource type.
// RHOAI creates a namespaced ServingRuntime per deployed model, so the count scales
// with the number of model deployments.
const servingRuntimeRecordsLimit = 100

type servingRuntime struct {
	// The namespace of a namespaced ServingRuntime is part of the archive path,
	// so it is not repeated here.
	Name                  string                    `json:"name"`
	SupportedModelFormats []servingRuntimeFormat    `json:"supportedModelFormats,omitempty"`
	Containers            []servingRuntimeContainer `json:"containers,omitempty"`
	MultiModel            *bool                     `json:"multiModel,omitempty"`
	ProtocolVersions      []string                  `json:"protocolVersions,omitempty"`
	WorkerSpec            *servingRuntimeWorkerSpec `json:"workerSpec,omitempty"`
}

type servingRuntimeFormat struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

type servingRuntimeContainer struct {
	Name  string   `json:"name,omitempty"`
	Image string   `json:"image,omitempty"`
	Args  []string `json:"args,omitempty"`
}

type servingRuntimeWorkerSpec struct {
	TensorParallelSize   int64 `json:"tensorParallelSize,omitempty"`
	PipelineParallelSize int64 `json:"pipelineParallelSize,omitempty"`
}

// GatherServingRuntimes Collects ServingRuntime resources from the KServe operator
// across all namespaces. Only selected non-sensitive fields are collected: supported
// model formats, container name/image/args, multi-model flag, protocol versions and
// worker parallelism.
//
// The containers env field is never collected as it commonly holds tokens and API keys.
// Values of container args are anonymized when the flag name looks sensitive or the
// value looks like a URI, since args may carry tokens or object storage paths.
//
// ### API Reference
// - https://kserve.github.io/website/docs/reference/crd-api#servingruntime
//
// ### Sample data
// - docs/insights-archive-sample/config/serving.kserve.io/kserve-demo/servingruntimes.json
//
// ### Location in archive
// - `config/serving.kserve.io/{namespace}/servingruntimes.json`
//
// ### Config ID
// `clusterconfig/serving_runtimes`
//
// ### Released version
// - 5.1
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherServingRuntimes(ctx context.Context) ([]record.Record, []error) {
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherServingRuntimes(ctx, dynamicClient)
}

func gatherServingRuntimes(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	list, err := dynamicClient.Resource(servingRuntimeGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	if len(list.Items) == 0 {
		return nil, nil
	}

	items, errs := limitServingRuntimes(list.Items, servingRuntimeGVR)

	runtimesByNamespace := make(map[string][]servingRuntime)
	for i := range items {
		namespace := items[i].GetNamespace()
		runtimesByNamespace[namespace] = append(runtimesByNamespace[namespace], extractServingRuntime(&items[i]))
	}

	namespaces := make([]string, 0, len(runtimesByNamespace))
	for namespace := range runtimesByNamespace {
		namespaces = append(namespaces, namespace)
	}
	sort.Strings(namespaces)

	records := make([]record.Record, 0, len(namespaces))
	for _, namespace := range namespaces {
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/serving.kserve.io/%s/servingruntimes", namespace),
			Item: record.JSONMarshaller{Object: runtimesByNamespace[namespace]},
		})
	}

	return records, errs
}

// extractServingRuntimes converts the unstructured runtimes into the gathered form,
// capping the result at servingRuntimeRecordsLimit.
func extractServingRuntimes(items []unstructured.Unstructured,
	gvr schema.GroupVersionResource,
) ([]servingRuntime, []error) {
	limited, errs := limitServingRuntimes(items, gvr)

	runtimes := make([]servingRuntime, 0, len(limited))
	for i := range limited {
		runtimes = append(runtimes, extractServingRuntime(&limited[i]))
	}

	return runtimes, errs
}

// limitServingRuntimes caps the number of gathered runtimes at servingRuntimeRecordsLimit,
// reporting an error when the limit is exceeded.
func limitServingRuntimes(items []unstructured.Unstructured,
	gvr schema.GroupVersionResource,
) ([]unstructured.Unstructured, []error) {
	if len(items) <= servingRuntimeRecordsLimit {
		return items, nil
	}

	return items[:servingRuntimeRecordsLimit], []error{fmt.Errorf("found %d %s resources, limiting to %d",
		len(items), gvr.GroupResource(), servingRuntimeRecordsLimit)}
}

func extractServingRuntime(item *unstructured.Unstructured) servingRuntime {
	sr := servingRuntime{
		Name: item.GetName(),
	}

	if formats, err := utils.NestedSliceWrapper(item.Object, "spec", "supportedModelFormats"); err == nil {
		sr.SupportedModelFormats = extractModelFormats(formats)
	}
	if containers, err := utils.NestedSliceWrapper(item.Object, "spec", "containers"); err == nil {
		sr.Containers = extractServingRuntimeContainers(containers)
	}
	if v, ok, err := unstructured.NestedBool(item.Object, "spec", "multiModel"); ok && err == nil {
		sr.MultiModel = &v
	}
	if v, ok, err := unstructured.NestedStringSlice(item.Object, "spec", "protocolVersions"); ok && err == nil {
		sr.ProtocolVersions = v
	}

	tensor, tensorErr := utils.NestedInt64Wrapper(item.Object, "spec", "workerSpec", "tensorParallelSize")
	pipeline, pipelineErr := utils.NestedInt64Wrapper(item.Object, "spec", "workerSpec", "pipelineParallelSize")
	if tensorErr == nil || pipelineErr == nil {
		sr.WorkerSpec = &servingRuntimeWorkerSpec{
			TensorParallelSize:   tensor,
			PipelineParallelSize: pipeline,
		}
	}

	return sr
}

func extractModelFormats(formats []interface{}) []servingRuntimeFormat {
	var result []servingRuntimeFormat
	for _, f := range formats {
		formatMap, ok := f.(map[string]interface{})
		if !ok {
			continue
		}
		var format servingRuntimeFormat
		format.Name, _, _ = unstructured.NestedString(formatMap, "name")
		format.Version, _, _ = unstructured.NestedString(formatMap, "version")
		result = append(result, format)
	}
	return result
}

// extractServingRuntimeContainers reads the container name, image and args.
// The env field is deliberately never read - it commonly holds tokens and API keys.
func extractServingRuntimeContainers(containers []interface{}) []servingRuntimeContainer {
	var result []servingRuntimeContainer
	for _, c := range containers {
		containerMap, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		var container servingRuntimeContainer
		container.Name, _, _ = unstructured.NestedString(containerMap, "name")
		container.Image, _, _ = unstructured.NestedString(containerMap, "image")
		if args, found, err := unstructured.NestedStringSlice(containerMap, "args"); found && err == nil {
			container.Args = anonymizeArgs(args)
		}
		result = append(result, container)
	}
	return result
}

// anonymizeArgs obfuscates the value of command line arguments that may carry
// sensitive data. An argument is treated as sensitive when its flag name matches
// sensitiveFieldPattern, or when its value looks like a URI, which covers object
// storage locations such as --download-dir=s3://bucket/path.
func anonymizeArgs(args []string) []string {
	result := make([]string, 0, len(args))
	for _, arg := range args {
		name, value, found := strings.Cut(arg, "=")
		if !found {
			if isURILike(arg) {
				arg = anonymize.String(arg)
			}
			result = append(result, arg)
			continue
		}
		if sensitiveFieldPattern.MatchString(name) || isURILike(value) {
			value = anonymize.String(value)
		}
		result = append(result, name+"="+value)
	}
	return result
}

func isURILike(s string) bool {
	return strings.Contains(s, "://") || strings.Contains(s, "@")
}
