package conditional

import (
	"fmt"
	"testing"

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

type validationTestCase struct {
	Name   string
	Rules  []GatheringRule
	Errors []string
}

func Test_Validation_InvalidGatheringRules(t *testing.T) {
	var tooManyRules []GatheringRule
	for i := 0; i < 1024; i++ {
		tooManyRules = append(tooManyRules, GatheringRule{
			Conditions: []ConditionWithParams{
				{
					Type: AlertIsFiring,
					Params: AlertIsFiringConditionParams{
						Name: "test" + fmt.Sprint(i),
					},
				},
			},
			GatheringFunctions: map[GatheringFunctionName]interface{}{
				GatherImageStreamsOfNamespace: GatherImageStreamsOfNamespaceParams{
					Namespace: "openshift-something-" + fmt.Sprint(i),
				},
			},
		})
	}

	testCases := []validationTestCase{
		{
			Name:  "nil",
			Rules: nil,
			Errors: []string{
				`(root): Invalid type. Expected: array, given: null`,
			},
		},
		{
			Name: "repeating elements",
			Rules: []GatheringRule{
				{
					Conditions: []ConditionWithParams{
						{
							Type: AlertIsFiring,
							Params: AlertIsFiringConditionParams{
								Name: "test1",
							},
						},
					},
					GatheringFunctions: map[GatheringFunctionName]interface{}{
						GatherImageStreamsOfNamespace: GatherImageStreamsOfNamespaceParams{
							Namespace: "openshift-something",
						},
					},
				},
				{
					Conditions: []ConditionWithParams{
						{
							Type: AlertIsFiring,
							Params: AlertIsFiringConditionParams{
								Name: "test1",
							},
						},
					},
					GatheringFunctions: map[GatheringFunctionName]interface{}{
						GatherImageStreamsOfNamespace: GatherImageStreamsOfNamespaceParams{
							Namespace: "openshift-something",
						},
					},
				},
			},
			Errors: []string{
				`(root): array items[0,1] must be unique`,
			},
		},
		{
			Name:  "too many rules",
			Rules: tooManyRules,
			Errors: []string{
				`(root): Array must have at most 64 items`,
			},
		},
	}

	assertValidationTestCases(t, testCases)
}

func Test_Validation_InvalidConditions(t *testing.T) {
	testCases := []validationTestCase{
		{
			Name: "invalid condition types",
			Rules: []GatheringRule{{
				Conditions: []ConditionWithParams{
					{
						Type: "", // empty
					},
					{
						Type: "invalid condition type.",
					},
				},
			}},
			Errors: []string{
				`0.conditions.0: Must validate at least one schema (anyOf)`,
				`0.conditions.0.type: 0.conditions.0.type does not match: "alert_is_firing"`,
				`0.conditions.0.params: Invalid type. Expected: object, given: null`,
				`0.conditions.1: Must validate at least one schema (anyOf)`,
				`0.conditions.1.type: 0.conditions.1.type does not match: "alert_is_firing"`,
				`0.conditions.1.params: Invalid type. Expected: object, given: null`,
				`0.gathering_functions: Invalid type. Expected: object, given: null`,
			},
		},
		{
			Name: "invalid type for params",
			Rules: []GatheringRule{{
				Conditions: []ConditionWithParams{
					{
						Type:   AlertIsFiring,
						Params: "using incorrect type for params",
					},
				},
			}},
			Errors: []string{
				`0.conditions.0: Must validate at least one schema (anyOf)`,
				`0.conditions.0.params: Invalid type. Expected: object, given: string`,
				`0.gathering_functions: Invalid type. Expected: object, given: null`,
			},
		},
		{
			Name: "invalid name in AlertIsFiringConditionParams",
			Rules: []GatheringRule{{
				Conditions: []ConditionWithParams{
					{
						Type: AlertIsFiring,
						Params: AlertIsFiringConditionParams{
							Name: "contains invalid characters $^#!@$%&",
						},
					},
				},
			}},
			Errors: []string{
				`0.conditions.0: Must validate at least one schema (anyOf)`,
				`0.conditions.0.params.name: Does not match pattern '^[a-zA-Z0-9_]{1,128}$'`,
				`0.gathering_functions: Invalid type. Expected: object, given: null`,
			},
		},
		{
			Name: "empty name in AlertIsFiringConditionParams",
			Rules: []GatheringRule{{
				Conditions: []ConditionWithParams{
					{
						Type: AlertIsFiring,
						Params: AlertIsFiringConditionParams{
							Name: "", // empty
						},
					},
				},
			}},
			Errors: []string{
				`0.gathering_functions: Invalid type. Expected: object, given: null`,
				`0.conditions.0: Must validate at least one schema (anyOf)`,
				`0.conditions.0.params.name: Does not match pattern '^[a-zA-Z0-9_]{1,128}$'`,
			},
		},
		{
			Name: "name is too long in AlertIsFiringConditionParams",
			Rules: []GatheringRule{{
				Conditions: []ConditionWithParams{
					{
						Type: AlertIsFiring,
						Params: AlertIsFiringConditionParams{
							Name: rand.String(1024), // too long
						},
					},
				},
			}},
			Errors: []string{
				`0.conditions.0: Must validate at least one schema (anyOf)`,
				`0.conditions.0.params.name: Does not match pattern '^[a-zA-Z0-9_]{1,128}$'`,
				`0.gathering_functions: Invalid type. Expected: object, given: null`,
			},
		},
	}

	assertValidationTestCases(t, testCases)
}

func Test_Validation_InvalidGatheringFunctions(t *testing.T) { //nolint:funlen
	var emptyStruct struct{}

	testCases := []validationTestCase{
		{
			Name: "invalid function names",
			Rules: []GatheringRule{
				{
					Conditions: []ConditionWithParams{},
					GatheringFunctions: map[GatheringFunctionName]interface{}{
						"":                      emptyStruct,
						"invalid function name": emptyStruct,
					},
				},
			},
			Errors: []string{
				`0.gathering_functions: Additional property  is not allowed`,
				`0.gathering_functions: Additional property invalid function name is not allowed`,
			},
		},
		{
			Name: "invalid params type",
			Rules: []GatheringRule{
				{
					Conditions: []ConditionWithParams{},
					GatheringFunctions: map[GatheringFunctionName]interface{}{
						GatherLogsOfNamespace: emptyStruct,
						GatherImageStreamsOfNamespace: GatherLogsOfNamespaceParams{
							Namespace: "openshift-something",
							TailLines: 1,
						},
					},
				},
			},
			Errors: []string{
				`0.gathering_functions.logs_of_namespace: namespace is required`,
				`0.gathering_functions.logs_of_namespace: tail_lines is required`,
			},
		},
		{
			Name: "invalid params",
			Rules: []GatheringRule{
				{
					Conditions: []ConditionWithParams{},
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
			},
			Errors: []string{
				`0.gathering_functions.logs_of_namespace.namespace: Does not match pattern '^openshift-[a-zA-Z0-9_.-]{1,128}$'`,
				`0.gathering_functions.logs_of_namespace.tail_lines: Must be greater than or equal to 1`,
				`0.gathering_functions.image_streams_of_namespace.namespace: Does not match pattern '^openshift-[a-zA-Z0-9_.-]{1,128}$'`,
			},
		},
		{
			Name: "invalid params 2",
			Rules: []GatheringRule{
				{
					Conditions: []ConditionWithParams{},
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
			},
			Errors: []string{
				`0.gathering_functions.image_streams_of_namespace.namespace: Does not match pattern '^openshift-[a-zA-Z0-9_.-]{1,128}$'`,
				`0.gathering_functions.logs_of_namespace.namespace: Does not match pattern '^openshift-[a-zA-Z0-9_.-]{1,128}$'`,
				`0.gathering_functions.logs_of_namespace.tail_lines: Must be less than or equal to 4096`,
			},
		},
		{
			Name: "invalid params 3",
			Rules: []GatheringRule{
				{
					Conditions: []ConditionWithParams{},
					GatheringFunctions: map[GatheringFunctionName]interface{}{
						GatherLogsOfNamespace: GatherLogsOfNamespaceParams{
							Namespace: rand.String(9999), // too long
							TailLines: 1024,
						},
					},
				},
			},
			Errors: []string{
				`0.gathering_functions.logs_of_namespace.namespace: Does not match pattern '^openshift-[a-zA-Z0-9_.-]{1,128}$'`,
			},
		},
		{
			Name: "empty GatheringFunctions",
			Rules: []GatheringRule{
				{
					Conditions:         []ConditionWithParams{},
					GatheringFunctions: nil,
				},
			},
			Errors: []string{
				`0.gathering_functions: Invalid type. Expected: object, given: null`,
			},
		},
		{
			Name: "empty GatheringFunctions",
			Rules: []GatheringRule{
				{
					Conditions:         []ConditionWithParams{},
					GatheringFunctions: map[GatheringFunctionName]interface{}{},
				},
			},
			Errors: []string{
				`0.gathering_functions: Must have at least 1 properties`,
			},
		},
	}

	assertValidationTestCases(t, testCases)
}

func assertValidationTestCases(t *testing.T, validationTestCases []validationTestCase) {
	for _, testCase := range validationTestCases {
		errs := validateGatheringRules(testCase.Rules)
		assertErrsMatchStrings(t, errs, testCase.Errors, testCase.Name)
	}
}

func assertErrsMatchStrings(t *testing.T, errs []error, expectedStrings []string, message string) {
	var errStrings []string
	for _, err := range errs {
		errStrings = append(errStrings, err.Error())
	}

	assert.ElementsMatchf(t, expectedStrings, errStrings, message)
}
