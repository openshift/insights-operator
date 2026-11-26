package clusterconfig

import (
	"context"
	"fmt"
	"regexp"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

// otlpExporterNameRegex matches OTLP exporter config keys, e.g. "otlp", "otlpgrpc",
// "otlphttp", "otlp_grpc", "otlp_http", and their "/name" variants such as "otlpgrpc/myexporter".
var otlpExporterNameRegex = regexp.MustCompile(`^otlp_?(grpc|http)?(/.+)?$`)

// GatherOpenTelemetryCollectors collects up to 5 `opentelemetrycollectors.opentelemetry.io` custom resources
// installed in the cluster.
//
// Only the "service" subsection of each resource's spec.config is retained, plus the (anonymized)
// endpoint of any OTLP exporter (keys matching otlp, otlpgrpc, otlphttp, otlp_grpc, otlp_http and
// their "/name" variants, e.g. otlpgrpc/myexporter). All other exporter/receiver configuration is
// omitted to avoid collecting sensitive data.
//
// ### API Reference
// - https://github.com/open-telemetry/opentelemetry-operator/blob/main/apis/v1beta1/opentelemetrycollector_types.go
//
// ### Sample data
// - docs/insights-archive-sample/config/opentelemetry/example-namespace/otel.json
//
// ### Location in archive
// - `config/opentelemetry/{namespace}/{name}.json`
//
// ### Config ID
// `clusterconfig/opentelemetry_collectors`
//
// ### Released version
// - 4.22
//
// ### Backported versions
// TBD
//
// ### Changes
// - CCXDEV-16802: added the (anonymized) OTLP exporter endpoint to spec.config.exporters
func (g *Gatherer) GatherOpenTelemetryCollectors(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherOpenTelemetryCollectors(ctx, gatherDynamicClient)
}

// cleanCollectorSpecConfig function parses the spec.config field
// and removes any possible private data getting only the "service" configuration
// and the endpoint of any OTLP exporter (anonymized)
func cleanCollectorSpecConfig(item *unstructured.Unstructured) error {
	// the last-applied-configuration annotation holds a full copy of the original spec.config,
	// including unanonymized OTLP exporter endpoints, so it must be dropped regardless of
	// whether spec.config itself is present below
	item.Object = removeFields(item.Object, [][]string{
		{"metadata", "annotations", "kubectl.kubernetes.io/last-applied-configuration"},
	})

	specConfig, found, err := unstructured.NestedMap(item.Object, "spec", "config")
	if err != nil {
		return err
	} else if !found {
		// skipping due to the lack of target data structure for this item
		return nil
	}

	// instead of dynamically remove every key, set only the desired one
	cleanConfig := make(map[string]interface{})
	if _, exists := specConfig["service"]; exists {
		cleanConfig["service"] = specConfig["service"]
	}

	if otlpExporters := otlpExportersEndpoints(specConfig["exporters"]); len(otlpExporters) > 0 {
		cleanConfig["exporters"] = otlpExporters
	}

	if err := unstructured.SetNestedField(item.Object, cleanConfig, "spec", "config"); err != nil {
		return err
	}

	return nil
}

// otlpExportersEndpoints extracts the (anonymized) endpoint of every OTLP exporter found in the
// given exporters map, keyed by exporter name (e.g. "otlp", "otlpgrpc/myexporter").
// Non-OTLP exporters and any field other than "endpoint" are dropped.
func otlpExportersEndpoints(exporters interface{}) map[string]interface{} {
	exportersMap, ok := exporters.(map[string]interface{})
	if !ok {
		return nil
	}

	result := make(map[string]interface{})
	for name, exporterCfg := range exportersMap {
		if !otlpExporterNameRegex.MatchString(name) {
			continue
		}

		cfgMap, ok := exporterCfg.(map[string]interface{})
		if !ok {
			continue
		}

		endpoint, ok := cfgMap["endpoint"].(string)
		if !ok {
			continue
		}

		result[name] = map[string]interface{}{
			"endpoint": anonymize.URL(endpoint),
		}
	}

	return result
}

func gatherOpenTelemetryCollectors(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	collectorsList, err := dynamicClient.Resource(openTelemetryCollectorResource).List(ctx, metav1.ListOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// fast exit with no error if no CRs were found
			return nil, nil
		}
		klog.V(2).Infof("unable to list %s resource due to: %s", openTelemetryCollectorResource, err)
		return nil, []error{err}
	}

	const limit = 5
	var records = make([]record.Record, 0, limit)
	var errs []error
	for i := range collectorsList.Items {
		item := &collectorsList.Items[i]

		if err := cleanCollectorSpecConfig(item); err != nil {
			errs = append(errs, err)
			continue
		}

		records = append(records, record.Record{
			Name: fmt.Sprintf("config/opentelemetry/%s/%s",
				item.GetNamespace(),
				item.GetName()),
			Item: record.ResourceMarshaller{Resource: item},
		})

		if len(records) >= limit {
			klog.Infof("the limit (%d) of gathered OpenTelemetryCollectors resources was reached", limit)
			break
		}
	}

	return records, errs
}
