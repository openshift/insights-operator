package clusterconfig

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/openshift/insights-operator/pkg/record"
)

func Test_GatherSubscriptions(t *testing.T) {
	// Sample subscription based on real gathered data
	subscriptionWithStatusYAML := `
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  creationTimestamp: "2026-02-18T08:53:55Z"
  generation: 1
  labels:
    operators.coreos.com/community-kubevirt-hyperconverged.kubevirt-hyperconverged: ""
  name: community-kubevirt-hyperconverged
  namespace: kubevirt-hyperconverged
  resourceVersion: "12345"
  uid: "00000000-0000-0000-0000-000000000000"
spec:
  channel: stable
  installPlanApproval: Automatic
  name: community-kubevirt-hyperconverged
  source: community-operators
  sourceNamespace: openshift-marketplace
  startingCSV: kubevirt-hyperconverged-operator.v1.16.0
status:
  catalogHealth:
  - catalogSourceRef:
      apiVersion: operators.coreos.com/v1alpha1
      kind: CatalogSource
      name: community-operators
      namespace: openshift-marketplace
    healthy: true
  conditions:
  - lastTransitionTime: "2026-02-18T08:54:00Z"
    message: all available catalogsources are healthy
    reason: AllCatalogSourcesHealthy
    status: "False"
    type: CatalogSourcesUnhealthy
  currentCSV: kubevirt-hyperconverged-operator.v1.16.0
  installedCSV: kubevirt-hyperconverged-operator.v1.16.0
  installplan:
    apiVersion: operators.coreos.com/v1alpha1
    kind: InstallPlan
    name: install-xyz789
    namespace: kubevirt-hyperconverged
  state: AtLatestKnown
`

	// Minimal subscription without status
	subscriptionMinimalYAML := `
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: test-operator
  namespace: openshift-operators
spec:
  channel: alpha
  name: test-operator
  source: test-catalog
  sourceNamespace: olm
`

	tests := []struct {
		name               string
		subscriptionYAMLs  []string
		totalRecords       int
		expectedRecordName string
		expectedError      bool
	}{
		{
			name:               "single subscription with status field that should be removed",
			subscriptionYAMLs:  []string{subscriptionWithStatusYAML},
			totalRecords:       1,
			expectedRecordName: "config/subscriptions/community-kubevirt-hyperconverged",
			expectedError:      false,
		},
		{
			name: "multiple subscriptions from different namespaces",
			subscriptionYAMLs: []string{
				subscriptionWithStatusYAML,
				subscriptionMinimalYAML,
			},
			totalRecords:  2,
			expectedError: false,
		},
		{
			name:              "no subscriptions available",
			subscriptionYAMLs: []string{},
			totalRecords:      0,
			expectedError:     false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gvr := schema.GroupVersionResource{
				Group:    "operators.coreos.com",
				Version:  "v1alpha1",
				Resource: "subscriptions",
			}

			dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
				runtime.NewScheme(),
				map[schema.GroupVersionResource]string{
					gvr: "SubscriptionsList",
				},
			)

			// Create subscription resources from YAML
			for _, subscriptionYAML := range tt.subscriptionYAMLs {
				decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
				subscription := &unstructured.Unstructured{}

				_, _, err := decUnstructured.Decode([]byte(subscriptionYAML), nil, subscription)
				assert.NoError(t, err, "unable to decode subscription")

				namespace := subscription.GetNamespace()
				_, err = dynamicClient.Resource(gvr).Namespace(namespace).Create(
					context.Background(),
					subscription,
					metav1.CreateOptions{},
				)
				assert.NoError(t, err, "unable to create fake subscription resource")
			}

			// Call the gatherer function
			records, errs := gatherSubscriptions(context.Background(), dynamicClient)

			// Verify errors
			if tt.expectedError {
				assert.NotEmpty(t, errs, "expected error but got none")
			} else {
				assert.Empty(t, errs, "unexpected errors")
			}

			// Verify record count
			assert.Equal(t, tt.totalRecords, len(records), "unexpected number of records")

			// Verify record name if specified
			if tt.expectedRecordName != "" && len(records) > 0 {
				assert.Equal(t, tt.expectedRecordName, records[0].Name, "unexpected record name")
			}

			// Verify that status field is removed from all records
			for i, rec := range records {
				marshaller, _ := rec.Item.(record.ResourceMarshaller)

				data, err := marshaller.Marshal()
				assert.NoError(t, err, "failed to marshal record %d", i)

				var result map[string]interface{}
				err = json.Unmarshal(data, &result)
				assert.NoError(t, err, "failed to unmarshal record %d data", i)

				// Critical check: status field must be removed
				_, hasStatus := result["status"]
				assert.False(t, hasStatus, "record %d still contains status field, but it should be removed", i)

				// Verify required fields are still present
				_, hasSpec := result["spec"]
				assert.True(t, hasSpec, "record %d is missing spec field", i)

				_, hasMetadata := result["metadata"]
				assert.True(t, hasMetadata, "record %d is missing metadata field", i)

				_, hasAPIVersion := result["apiVersion"]
				assert.True(t, hasAPIVersion, "record %d is missing apiVersion field", i)

				_, hasKind := result["kind"]
				assert.True(t, hasKind, "record %d is missing kind field", i)
			}
		})
	}
}

func Test_GatherSubscriptions_NotFound(t *testing.T) {
	t.Parallel()

	gvr := schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "subscriptions",
	}

	// Create a dynamic client with the resource registered but no subscriptions created
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			gvr: "SubscriptionsList",
		},
	)

	// Don't create any subscription resources - this will return an empty list
	records, errs := gatherSubscriptions(context.Background(), dynamicClient)

	// When no subscriptions exist, gatherer should return empty records and no errors
	assert.Empty(t, records, "expected 0 records when no subscriptions exist")
	assert.Empty(t, errs, "expected 0 errors when no subscriptions exist")
}

func Test_GatherSubscriptions_StatusFieldRemoval(t *testing.T) {
	t.Parallel()

	// This test specifically validates that the status field is removed
	subscriptionYAML := `
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: status-removal-test
  namespace: test-namespace
spec:
  channel: stable
  name: test-operator
  source: test-source
  sourceNamespace: test-ns
status:
  currentCSV: test-operator.v1.0.0
  installedCSV: test-operator.v1.0.0
  state: AtLatestKnown
  conditions:
  - type: CatalogSourcesUnhealthy
    status: "False"
`

	gvr := schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "subscriptions",
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			gvr: "SubscriptionsList",
		},
	)

	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	subscription := &unstructured.Unstructured{}

	_, _, err := decUnstructured.Decode([]byte(subscriptionYAML), nil, subscription)
	assert.NoError(t, err, "failed to decode subscription YAML")

	// Verify the test data has a status field before gathering
	_, statusExists, err := unstructured.NestedFieldNoCopy(subscription.Object, "status")
	assert.NoError(t, err, "error checking status field")
	assert.True(t, statusExists, "test subscription should have status field before processing")

	_, err = dynamicClient.Resource(gvr).Namespace("test-namespace").Create(
		context.Background(),
		subscription,
		metav1.CreateOptions{},
	)
	assert.NoError(t, err, "failed to create fake subscription resource")

	records, errs := gatherSubscriptions(context.Background(), dynamicClient)

	assert.Len(t, records, 1, "expected exactly 1 record")
	assert.Empty(t, errs, "expected no errors")

	// Marshal and verify the gathered data
	marshaller, ok := records[0].Item.(record.ResourceMarshaller)
	assert.True(t, ok, "record is not of type ResourceMarshaller")

	data, err := marshaller.Marshal()
	assert.NoError(t, err, "failed to marshal record")

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	assert.NoError(t, err, "failed to unmarshal record data")

	// Primary assertion: status field must be removed
	_, hasStatus := result["status"]
	assert.False(t, hasStatus, "status field was not removed from gathered subscription data")

	// Verify spec data is preserved
	spec, hasSpec := result["spec"].(map[string]interface{})
	assert.True(t, hasSpec, "spec field must be present")

	// Verify specific spec fields
	channel, hasChannel := spec["channel"].(string)
	assert.True(t, hasChannel, "channel field should exist in spec")
	assert.Equal(t, "stable", channel, "unexpected channel value")

	name, hasName := spec["name"].(string)
	assert.True(t, hasName, "name field should exist in spec")
	assert.Equal(t, "test-operator", name, "unexpected name value")

	// Verify metadata is preserved
	metadata, hasMetadata := result["metadata"].(map[string]interface{})
	assert.True(t, hasMetadata, "metadata field must be present")

	metadataName, hasMetadataName := metadata["name"].(string)
	assert.True(t, hasMetadataName, "metadata.name should exist")
	assert.Equal(t, "status-removal-test", metadataName, "unexpected metadata.name value")
}
