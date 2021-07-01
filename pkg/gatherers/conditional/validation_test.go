package conditional

import (
	"strings"
	"testing"

	"github.com/go-playground/validator"
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

func Test_Validation_InvalidConditions(t *testing.T) {
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
	assert.Len(t, errs, 1)

	assertValidationErrors(t, errs[0], []string{
		"Key: 'GatheringRule.Conditions[3].Params.Name' Error:Field validation for 'Name' failed on the 'alphanum' tag",
		"Key: 'GatheringRule.Conditions[4].Params.Name' Error:Field validation for 'Name' failed on the 'min' tag",
		"Key: 'GatheringRule.Conditions[5].Params.Name' Error:Field validation for 'Name' failed on the 'max' tag",
		"Key: 'GatheringRule.Conditions[].Type' Error:Field validation for 'Conditions[].Type' failed on the 'is_valid' tag",
		"Key: 'GatheringRule.Conditions[].Type' Error:Field validation for 'Conditions[].Type' failed on the 'is_valid' tag",
		"Key: 'GatheringRule.Conditions[].Params' Error:Field validation for 'Conditions[].Params' failed on the 'is_valid_type' tag",
		"Key: 'GatheringRule.GatheringFunctions' Error:Field validation for 'GatheringFunctions' failed on the 'not_empty' tag",
	})
}

func Test_Validation_InvalidGatheringFunctions(t *testing.T) {
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
	expectedErrors := [][]string{
		{
			"Key: 'GatheringRule.GatheringFunctions[].Name' Error:Field validation for 'GatheringFunctions[].Name' failed on the 'is_valid' tag",
			"Key: 'GatheringRule.GatheringFunctions[].Name' Error:Field validation for 'GatheringFunctions[].Name' failed on the 'is_valid' tag",
		},
		{
			"Key: 'GatheringRule.GatheringFunctions[].Params' Error:Field validation for 'GatheringFunctions[].Params' failed on the 'is_valid_type' tag",
			"Key: 'GatheringRule.GatheringFunctions[].Params' Error:Field validation for 'GatheringFunctions[].Params' failed on the 'is_valid_type' tag",
		},
		{
			"Key: 'GatheringRule.GatheringFunctions[logs_of_namespace].Namespace' Error:Field validation for 'Namespace' failed on the 'openshift_namespace' tag",
			"Key: 'GatheringRule.GatheringFunctions[logs_of_namespace].TailLines' Error:Field validation for 'TailLines' failed on the 'min' tag",
			"Key: 'GatheringRule.GatheringFunctions[image_streams_of_namespace].Namespace' Error:Field validation for 'Namespace' failed on the 'openshift_namespace' tag",
		},
		{
			"Key: 'GatheringRule.GatheringFunctions[logs_of_namespace].Namespace' Error:Field validation for 'Namespace' failed on the 'openshift_namespace' tag",
			"Key: 'GatheringRule.GatheringFunctions[logs_of_namespace].TailLines' Error:Field validation for 'TailLines' failed on the 'max' tag",
			"Key: 'GatheringRule.GatheringFunctions[image_streams_of_namespace].Namespace' Error:Field validation for 'Namespace' failed on the 'openshift_namespace' tag",
		},
		{
			"Key: 'GatheringRule.GatheringFunctions[logs_of_namespace].Namespace' Error:Field validation for 'Namespace' failed on the 'openshift_namespace' tag",
		},
		{
			"Key: 'GatheringRule.GatheringFunctions' Error:Field validation for 'GatheringFunctions' failed on the 'not_empty' tag",
		},
		{
			"Key: 'GatheringRule.GatheringFunctions' Error:Field validation for 'GatheringFunctions' failed on the 'not_empty' tag",
		},
	}

	errs := validateGatheringRules(gatheringRules)
	assert.Len(t, errs, len(gatheringRules))

	for i := 0; i < len(gatheringRules); i++ {
		assertValidationErrors(t, errs[i], expectedErrors[i])
	}
}

func assertValidationErrors(t *testing.T, err error, expectedErrors []string) {
	validationErrors, ok := err.(validator.ValidationErrors)
	assert.True(t, ok)

	errs := strings.Split(validationErrors.Error(), "\n")
	assert.ElementsMatch(t, errs, expectedErrors)
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
