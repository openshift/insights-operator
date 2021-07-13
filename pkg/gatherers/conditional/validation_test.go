package conditional

import (
	"sort"
	"testing"

	"github.com/openshift/insights-operator/pkg/utils"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/rand"
)

func Test_Validation(t *testing.T) {
	gatheringRules := []GatheringRule{
		{
			Conditions: []ConditionWithParams{
				{
					Type: AlertIsFiring,
					Params: AlertIsFiringConditionParams{
						Name: "test1",
					},
				},
				{
					Type: AlertIsFiring,
					Params: AlertIsFiringConditionParams{
						Name: "test2",
					},
				},
			},
			GatheringFunctions: map[GatheringFunctionName]interface{}{
				GatherLogsOfNamespace: GatherLogsOfNamespaceParams{
					Namespace: "openshift-something",
					TailLines: 1,
				},
				GatherImageStreamsOfNamespace: GatherImageStreamsOfNamespaceParams{
					Namespace: "openshift-something",
				},
			},
		},
	}
	errs := validateGatheringRules(gatheringRules)
	assert.Empty(t, errs)
}

func Test_Validation_InvalidConditions(t *testing.T) { //nolint:funlen
	gatheringRules := []GatheringRule{
		{
			Conditions: []ConditionWithParams{
				{
					Type: "", // empty
				},
				{
					Type: "invalid condition type.",
				},
				{
					Type:   AlertIsFiring,
					Params: "using incorrect type for params",
				},
				{
					Type: AlertIsFiring,
					Params: AlertIsFiringConditionParams{
						Name: "contains invalid characters $^#!@$%&",
					},
				},
				{
					Type: AlertIsFiring,
					Params: AlertIsFiringConditionParams{
						Name: "", // empty
					},
				},
				{
					Type: AlertIsFiring,
					Params: AlertIsFiringConditionParams{
						Name: rand.String(1024), // too long
					},
				},
			},
			GatheringFunctions: nil,
		},
	}
	errs := validateGatheringRules(gatheringRules)
	assert.Len(t, errs, 7)

	assertValidationErrors(t, errs, []string{
		`{
			"actual_tag":"alphanum",
			"field":"Name",
			"kind":"string",
			"namespace":"GatheringRule.Conditions[3].Params.Name",
			"param":"",
			"struct_field":"Name",
			"struct_namespace":"GatheringRule.Conditions[3].Params.Name",
			"tag":"alphanum",
			"type":"string"
		}`,
		`{
			"actual_tag":"min",
			"field":"Name",
			"kind":"string",
			"namespace":"GatheringRule.Conditions[4].Params.Name",
			"param":"1",
			"struct_field":"Name",
			"struct_namespace":"GatheringRule.Conditions[4].Params.Name",
			"tag":"min",
			"type":"string"
		}`,
		`{
			"actual_tag":"max",
			"field":"Name",
			"kind":"string",
			"namespace":"GatheringRule.Conditions[5].Params.Name",
			"param":"128",
			"struct_field":"Name",
			"struct_namespace":"GatheringRule.Conditions[5].Params.Name",
			"tag":"max",
			"type":"string"
		}`,
		`{
			"actual_tag":"is_valid",
			"field":"Conditions[].Type",
			"kind":"string",
			"namespace":"GatheringRule.Conditions[].Type",
			"param":"invalid value for conditional.ConditionType",
			"struct_field":"Conditions[].Type",
			"struct_namespace":"GatheringRule.Conditions[].Type",
			"tag":"is_valid",
			"type":"conditional.ConditionType"
		}`,
		`{
			"actual_tag":"is_valid",
			"field":"Conditions[].Type",
			"kind":"string",
			"namespace":"GatheringRule.Conditions[].Type",
			"param":"invalid value for conditional.ConditionType",
			"struct_field":"Conditions[].Type",
			"struct_namespace":"GatheringRule.Conditions[].Type",
			"tag":"is_valid",
			"type":"conditional.ConditionType"
		}`,
		`{
			"actual_tag":"is_valid_type",
			"field":"Conditions[].Params",
			"kind":"string",
			"namespace":"GatheringRule.Conditions[].Params",
			"param":"params cannot be string for conditional.ConditionType",
			"struct_field":"Conditions[].Params",
			"struct_namespace":"GatheringRule.Conditions[].Params",
			"tag":"is_valid_type",
			"type":"string"
		}`,
		`{
			"actual_tag":"not_empty",
			"field":"GatheringFunctions",
			"kind":"map",
			"namespace":"GatheringRule.GatheringFunctions",
			"param":"",
			"struct_field":"GatheringFunctions",
			"struct_namespace":"GatheringRule.GatheringFunctions",
			"tag":"not_empty",
			"type":"map[conditional.GatheringFunctionName]interface {}"
		}`,
	})
}

func Test_Validation_InvalidGatheringFunctions(t *testing.T) { //nolint:funlen
	var emptyStruct struct{}

	gatheringRules := []GatheringRule{
		{ // invalid function names
			GatheringFunctions: map[GatheringFunctionName]interface{}{
				"":                      emptyStruct,
				"invalid function name": emptyStruct,
			},
		},
		{ // invalid params types
			GatheringFunctions: map[GatheringFunctionName]interface{}{
				GatherLogsOfNamespace: emptyStruct,
				GatherImageStreamsOfNamespace: GatherLogsOfNamespaceParams{
					Namespace: "openshift-something",
					TailLines: 1,
				},
			},
		},
		{ // invalid params
			GatheringFunctions: map[GatheringFunctionName]interface{}{
				GatherLogsOfNamespace: GatherLogsOfNamespaceParams{
					Namespace: "", // empty
					TailLines: 0,  // too small
				},
				GatherImageStreamsOfNamespace: GatherImageStreamsOfNamespaceParams{
					Namespace: "", // empty
				},
			},
		},
		{ // invalid params
			GatheringFunctions: map[GatheringFunctionName]interface{}{
				GatherLogsOfNamespace: GatherLogsOfNamespaceParams{
					Namespace: "not-openshift-namespace",
					TailLines: 999999999, // too big
				},
				GatherImageStreamsOfNamespace: GatherImageStreamsOfNamespaceParams{
					Namespace: "openshift-invalid-namespace-name#@$^@#$&!#$%@!#$%@#$", // invalid characters
				},
			},
		},
		{ // invalid params
			GatheringFunctions: map[GatheringFunctionName]interface{}{
				GatherLogsOfNamespace: GatherLogsOfNamespaceParams{
					Namespace: rand.String(9999), // too long
					TailLines: 1024,
				},
			},
		},
		{ // empty
			GatheringFunctions: nil,
		},
		{ // empty
			GatheringFunctions: map[GatheringFunctionName]interface{}{},
		},
	}
	expectedErrors := []string{
		`{
			"actual_tag":"is_valid",
			"field":"GatheringFunctions[].Name",
			"kind":"string",
			"namespace":"GatheringRule.GatheringFunctions[].Name",
			"param":"invalid value for conditional.GatheringFunctionName",
			"struct_field":"GatheringFunctions[].Name",
			"struct_namespace":"GatheringRule.GatheringFunctions[].Name",
			"tag":"is_valid",
			"type":"conditional.GatheringFunctionName"
		}`,
		`{
			"actual_tag":"startswith",
			"field":"Namespace",
			"kind":"string",
			"namespace":"GatheringRule.GatheringFunctions[logs_of_namespace].Namespace",
			"param":"openshift-",
			"struct_field":"Namespace",
			"struct_namespace":"GatheringRule.GatheringFunctions[logs_of_namespace].Namespace",
			"tag":"openshift_namespace",
			"type":"string"
		}`,
		`{
			"actual_tag":"is_valid",
			"field":"GatheringFunctions[].Name",
			"kind":"string",
			"namespace":"GatheringRule.GatheringFunctions[].Name",
			"param":"invalid value for conditional.GatheringFunctionName",
			"struct_field":"GatheringFunctions[].Name",
			"struct_namespace":"GatheringRule.GatheringFunctions[].Name",
			"tag":"is_valid",
			"type":"conditional.GatheringFunctionName"
		}`,
		`{
			"actual_tag":"is_valid_type",
			"field":"GatheringFunctions[].Params",
			"kind":"struct",
			"namespace":"GatheringRule.GatheringFunctions[].Params",
			"param":"params cannot be struct {} for conditional.GatheringFunctionName",
			"struct_field":"GatheringFunctions[].Params",
			"struct_namespace":"GatheringRule.GatheringFunctions[].Params",
			"tag":"is_valid_type",
			"type":"struct {}"
		}`,
		`{
			"actual_tag":"is_valid_type",
			"field":"GatheringFunctions[].Params",
			"kind":"struct",
			"namespace":"GatheringRule.GatheringFunctions[].Params",
			"param":"params cannot be conditional.GatherLogsOfNamespaceParams for conditional.GatheringFunctionName",
			"struct_field":"GatheringFunctions[].Params",
			"struct_namespace":"GatheringRule.GatheringFunctions[].Params",
			"tag":"is_valid_type",
			"type":"conditional.GatherLogsOfNamespaceParams"
		}`,
		`{
			"actual_tag":"min",
			"field":"Namespace",
			"kind":"string",
			"namespace":"GatheringRule.GatheringFunctions[logs_of_namespace].Namespace",
			"param":"1",
			"struct_field":"Namespace",
			"struct_namespace":"GatheringRule.GatheringFunctions[logs_of_namespace].Namespace",
			"tag":"openshift_namespace",
			"type":"string"
		}`,
		`{
			"actual_tag":"min",
			"field":"TailLines",
			"kind":"int64",
			"namespace":"GatheringRule.GatheringFunctions[logs_of_namespace].TailLines",
			"param":"1",
			"struct_field":"TailLines",
			"struct_namespace":"GatheringRule.GatheringFunctions[logs_of_namespace].TailLines",
			"tag":"min",
			"type":"int64"
		}`,
		`{
			"actual_tag":"min",
			"field":"Namespace",
			"kind":"string",
			"namespace":"GatheringRule.GatheringFunctions[image_streams_of_namespace].Namespace",
			"param":"1",
			"struct_field":"Namespace",
			"struct_namespace":"GatheringRule.GatheringFunctions[image_streams_of_namespace].Namespace",
			"tag":"openshift_namespace",
			"type":"string"
		}`,
		`{
			"actual_tag":"max",
			"field":"TailLines",
			"kind":"int64",
			"namespace":"GatheringRule.GatheringFunctions[logs_of_namespace].TailLines",
			"param":"4096",
			"struct_field":"TailLines",
			"struct_namespace":"GatheringRule.GatheringFunctions[logs_of_namespace].TailLines",
			"tag":"max",
			"type":"int64"
		}`,
		`{
			"actual_tag":"hostname",
			"field":"Namespace",
			"kind":"string",
			"namespace":"GatheringRule.GatheringFunctions[image_streams_of_namespace].Namespace",
			"param":"",
			"struct_field":"Namespace",
			"struct_namespace":"GatheringRule.GatheringFunctions[image_streams_of_namespace].Namespace",
			"tag":"openshift_namespace",
			"type":"string"
		}`,
		`{
			"actual_tag":"max",
			"field":"Namespace",
			"kind":"string",
			"namespace":"GatheringRule.GatheringFunctions[logs_of_namespace].Namespace",
			"param":"128",
			"struct_field":"Namespace",
			"struct_namespace":"GatheringRule.GatheringFunctions[logs_of_namespace].Namespace",
			"tag":"openshift_namespace",
			"type":"string"
		}`,
		`{
			"actual_tag":"not_empty",
			"field":"GatheringFunctions",
			"kind":"map",
			"namespace":"GatheringRule.GatheringFunctions",
			"param":"",
			"struct_field":"GatheringFunctions",
			"struct_namespace":"GatheringRule.GatheringFunctions",
			"tag":"not_empty",
			"type":"map[conditional.GatheringFunctionName]interface {}"
		}`,
		`{
			"actual_tag":"not_empty",
			"field":"GatheringFunctions",
			"kind":"map",
			"namespace":"GatheringRule.GatheringFunctions",
			"param":"",
			"struct_field":"GatheringFunctions",
			"struct_namespace":"GatheringRule.GatheringFunctions",
			"tag":"not_empty",
			"type":"map[conditional.GatheringFunctionName]interface {}"
		}`,
	}

	errs := validateGatheringRules(gatheringRules)
	assert.Len(t, errs, len(expectedErrors))

	assertValidationErrors(t, errs, expectedErrors)
}

func assertValidationErrors(t *testing.T, errs []error, expectedErrors []string) {
	errsStrings := utils.ErrorsToStrings(errs)
	sort.Strings(errsStrings)
	sort.Strings(expectedErrors)

	assert.Equal(t, len(errsStrings), len(expectedErrors))

	for i, actualErr := range errsStrings {
		expectedErr := expectedErrors[i]
		assert.JSONEq(t, expectedErr, actualErr)
	}
}

// Test_Validation_Workaround tests a workaround to not panic https://github.com/go-playground/validator/issues/789
func Test_Validation_Workaround(t *testing.T) {
	errs := validateGatheringRules([]GatheringRule{{
		GatheringFunctions: map[GatheringFunctionName]interface{}{
			"test": "",
		},
	}})
	assert.Len(t, errs, 1)
	assert.EqualError(t, errs[0], "gathering function params should be a struct, key is test")
}
