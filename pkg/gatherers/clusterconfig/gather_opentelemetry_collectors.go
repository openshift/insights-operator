package clusterconfig

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/record"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"
)

// GatherOpenTelemetryCollectors collects `opentelemetrycollectors.opentelemetry.io`
// TODO
//
// ### API Reference
// None
//
// ### Sample data
// - TODO
//
// ### Location in archive
// - `config/opentelemetry.io/opentelemetrycollectors/{namespace}/{name}.json` ??
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
// None
func (g *Gatherer) GatherOpenTelemetryCollectors(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherOpenTelemetryCollectors(ctx, gatherDynamicClient)
}

// parseCollectorSpecConfig function parses the spec.config field data in YAML format
// and removes any possible private data getting only the "service" configuration
func parseCollectorSpecConfig(item *unstructured.Unstructured) error {
	specConfig, found, err := unstructured.NestedString(item.Object, "spec", "config")
	if err != nil {
		return err
	} else if !found {
		// skipping due to the lack of target data structure for this item
		return nil
	}

	// easier than parsing everything and then removing unwanted data
	// it only parses the "service" field
	var serviceField struct {
		Data map[string]interface{} `json:"service"`
	}

	if err := yaml.Unmarshal([]byte(specConfig), &serviceField); err != nil {
		return err
	}

	// preparing the data to be added back, while keeping the parent fields
	parsedField := make(map[string]interface{})
	parsedField["service"] = serviceField.Data

	if err := unstructured.SetNestedField(item.Object, parsedField, "spec", "config"); err != nil {
		return err
	}

	return nil
}

func gatherOpenTelemetryCollectors(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	collectorsList, err := dynamicClient.Resource(openTelemetryCollectorResource).List(ctx, metav1.ListOptions{})
	if err != nil {
		// fast exit if no CRs were found
		if errors.IsNotFound(err) {
			return nil, nil
		}

		return nil, []error{err}
	}

	const limit = 5
	var records = make([]record.Record, 0, limit)
	var errs []error
	for i := range collectorsList.Items {
		item := &collectorsList.Items[i]

		if err := parseCollectorSpecConfig(item); err != nil {
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
			errs = append(errs,
				fmt.Errorf("limit %d for number of gathered OpenTelemetryCollectors resources exceeded", limit))
			break
		}
	}

	return records, errs
}
