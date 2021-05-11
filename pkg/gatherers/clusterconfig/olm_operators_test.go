package clusterconfig

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/openshift/insights-operator/pkg/record"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

//nolint: funlen
func Test_OLMOperators_Gather(t *testing.T) {
	var cases = []struct {
		testName            string
		olmOperatorFileName string
		csvFileName         string
		expectedError       error
		expecteOlmOperator  olmOperator
	}{
		{
			"All OLM operator data is available",
			"testdata/olm_operator_1.yaml",
			"testdata/csv_1.yaml",
			nil,
			olmOperator{
				Name:        "test-olm-operator",
				DisplayName: "Testing operator",
				Version:     "v1.2.3",
				Conditions: []interface{}{
					map[string]interface{}{
						"lastTransitionTime": "2021-03-02T08:52:24Z",
						"lastUpdateTime":     "2021-03-02T08:52:24Z",
						"message":            "requirements not yet checked",
						"phase":              "Pending",
						"reason":             "RequirementsUnknown",
					},
					map[string]interface{}{
						"lastTransitionTime": "2021-03-02T08:52:24Z",
						"lastUpdateTime":     "2021-03-02T08:52:24Z",
						"message":            "all requirements found, attempting install",
						"phase":              "InstallReady",
						"reason":             "AllRequirementsMet",
					},
				},
			},
		},
		{
			"Operator doesn't have CSV reference",
			"testdata/olm_operator_2.yaml",
			"testdata/csv_1.yaml",
			fmt.Errorf("cannot find \"status.components.refs\" in test-olm-operator-with-no-ref definition: key refs wasn't found in map[] "),
			olmOperator{
				Name: "test-olm-operator-with-no-ref",
			},
		},
		{
			"Operator CSV doesn't have the displayName",
			"testdata/olm_operator_1.yaml",
			"testdata/csv_2.yaml",
			fmt.Errorf("cannot read test-olm-operator.v1.2.3 ClusterServiceVersion attributes: key displayName wasn't found in map[] "),
			olmOperator{
				Name:    "test-olm-operator",
				Version: "v1.2.3",
			},
		},
		{
			"Operator with unrecognizable CSV version",
			"testdata/olm_operator_3.yaml",
			"testdata/csv_1.yaml",
			fmt.Errorf("clusterserviceversion \"name-without-version\" probably doesn't include version"),
			olmOperator{
				Name: "test-olm-operator-no-version",
			},
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			olmOpContent, err := readFromFile(tt.olmOperatorFileName)
			if err != nil {
				t.Fatal("test failed to read OLM operator data", err)
			}

			csvContent, err := readFromFile(tt.csvFileName)
			if err != nil {
				t.Fatal("test failed to read CSV ", err)
			}
			client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
				operatorGVR:              "OperatorsList",
				clusterServiceVersionGVR: "ClusterServiceVersionsList",
			})
			err = createUnstructuredResource(olmOpContent, client, operatorGVR)
			if err != nil {
				t.Fatal("cannot create OLM operator ", err)
			}
			err = createUnstructuredResource(csvContent, client, clusterServiceVersionGVR)
			if err != nil {
				t.Fatal("cannot create ClusterServiceVersion ", err)
			}

			ctx := context.Background()
			records, errs := gatherOLMOperators(ctx, client)
			if len(errs) > 0 {
				if errs[0].Error() != tt.expectedError.Error() {
					t.Fatalf("unexpected errors: %v", errs[0].Error())
				}
			}
			if len(records) != 1 {
				t.Fatalf("unexpected number or records %d", len(records))
			}
			ooa, ok := records[0].Item.(record.JSONMarshaller).Object.([]olmOperator)
			if !ok {
				t.Fatalf("returned item is not of type []olmOperator")
			}
			sameOp := reflect.DeepEqual(ooa[0], tt.expecteOlmOperator)
			if !sameOp {
				t.Fatalf("Gathered %s operator is not equal to expected %s ", ooa[0], tt.expecteOlmOperator)
			}
		})
	}
}

func createUnstructuredResource(content []byte, client *dynamicfake.FakeDynamicClient, gvr schema.GroupVersionResource) error {
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	unstructuredResource := &unstructured.Unstructured{}

	_, _, err := decUnstructured.Decode(content, nil, unstructuredResource)
	if err != nil {
		return err
	}

	_, err = client.Resource(gvr).Namespace("test-olm-operator").Create(context.Background(), unstructuredResource, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func readFromFile(filePath string) ([]byte, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}

	defer f.Close()

	content, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	return content, nil
}
