package conditional

import (
	"fmt"
	"strings"
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
					Alert: &AlertConditionParams{
						Name: "test1",
					},
				},
				{
					Type: AlertIsFiring,
					Alert: &AlertConditionParams{
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
					Alert: &AlertConditionParams{
						Name: "testInvalid" + fmt.Sprint(i),
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
				`there are no conditional rules`,
			},
		},
		{
			Name: "empty Rules",
			Rules: []GatheringRule{
				{},
			},
			Errors: []string{
				`0.conditions: Invalid type. Expected: array, given: null`,
				`0.gathering_functions: Invalid type. Expected: object, given: null`,
			},
		},
		{
			Name: "repeating elements",
			Rules: []GatheringRule{
				{
					Conditions: []ConditionWithParams{
						{
							Type: AlertIsFiring,
							Alert: &AlertConditionParams{
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
							Alert: &AlertConditionParams{
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

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			errs := validateGatheringRules(tc.Rules)
			assertErrsMatchStrings(t, errs, tc.Errors, tc.Name)
		})
	}
}

func Test_Validation_InvalidConditions(t *testing.T) {
	var tooManyConditions []ConditionWithParams
	for i := 0; i < 9; i++ {
		tooManyConditions = append(tooManyConditions, ConditionWithParams{
			Type: AlertIsFiring,
			Alert: &AlertConditionParams{
				Name: "test" + fmt.Sprint(i),
			},
		})
	}

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
				`0.conditions.0.type: 0.conditions.0.type does not match: "alert_is_firing"`,
				`0.conditions.0: Must validate at least one schema (anyOf)`,
				`0.conditions.0: alert is required`,
				`0.conditions.1.type: 0.conditions.1.type does not match: "alert_is_firing"`,
				`0.conditions.1: Must validate at least one schema (anyOf)`,
				`0.conditions.1: alert is required`,
				`0.gathering_functions: Invalid type. Expected: object, given: null`,
			},
		},
		{
			Name: "invalid type for params",
			Rules: []GatheringRule{{
				Conditions: []ConditionWithParams{
					{
						Type:  AlertIsFiring,
						Alert: nil,
					},
				},
			}},
			Errors: []string{
				`0.conditions.0: Must validate at least one schema (anyOf)`,
				`0.conditions.0: alert is required`,
				`0.gathering_functions: Invalid type. Expected: object, given: null`,
			},
		},
		{
			Name: "invalid name in AlertIsFiringConditionParams",
			Rules: []GatheringRule{{
				Conditions: []ConditionWithParams{
					{
						Type: AlertIsFiring,
						Alert: &AlertConditionParams{
							Name: "contains invalid characters $^#!@$%&",
						},
					},
				},
			}},
			Errors: []string{
				`0.conditions.0.alert.name: Does not match pattern '^[a-zA-Z0-9_]{1,128}$'`,
				`0.conditions.0: Must validate at least one schema (anyOf)`,
				`0.gathering_functions: Invalid type. Expected: object, given: null`,
			},
		},
		{
			Name: "empty name in AlertIsFiringConditionParams",
			Rules: []GatheringRule{{
				Conditions: []ConditionWithParams{
					{
						Type: AlertIsFiring,
						Alert: &AlertConditionParams{
							Name: "", // empty
						},
					},
				},
			}},
			Errors: []string{
				`0.conditions.0.alert.name: Does not match pattern '^[a-zA-Z0-9_]{1,128}$'`,
				`0.conditions.0: Must validate at least one schema (anyOf)`,
				`0.gathering_functions: Invalid type. Expected: object, given: null`,
			},
		},
		{
			Name: "name is too long in AlertIsFiringConditionParams",
			Rules: []GatheringRule{{
				Conditions: []ConditionWithParams{
					{
						Type: AlertIsFiring,
						Alert: &AlertConditionParams{
							Name: rand.String(1024), // too long
						},
					},
				},
			}},
			Errors: []string{
				`0.conditions.0.alert.name: Does not match pattern '^[a-zA-Z0-9_]{1,128}$'`,
				`0.conditions.0: Must validate at least one schema (anyOf)`,
				`0.gathering_functions: Invalid type. Expected: object, given: null`,
			},
		},
		{
			Name: "Cluster version cannot be empty",
			Rules: []GatheringRule{{
				Conditions: []ConditionWithParams{
					{
						Type: ClusterVersionMatches,
						ClusterVersionMatches: &ClusterVersionMatchesConditionParams{
							Version: "",
						},
					},
				},
			}},
			Errors: []string{
				`0.conditions.0.cluster_version_matches.version: String length must be greater than or equal to 1`,
				`0.conditions.0: Must validate at least one schema (anyOf)`,
				`0.gathering_functions: Invalid type. Expected: object, given: null`,
			},
		},
		{
			Name: "Cluster version cannot exceed 64 chars",
			Rules: []GatheringRule{{
				Conditions: []ConditionWithParams{
					{
						Type: ClusterVersionMatches,
						ClusterVersionMatches: &ClusterVersionMatchesConditionParams{
							Version: rand.String(65), // too long,
						},
					},
				},
			}},
			Errors: []string{
				`0.conditions.0.cluster_version_matches.version: String length must be less than or equal to 64`,
				`0.conditions.0: Must validate at least one schema (anyOf)`,
				`0.gathering_functions: Invalid type. Expected: object, given: null`,
			},
		},
		{
			Name: "Alert name is invalid",
			Rules: []GatheringRule{{
				Conditions: []ConditionWithParams{
					{
						Type: AlertIsFiring,
						Alert: &AlertConditionParams{
							Name: "??--..",
						},
					},
				},
			}},
			Errors: []string{
				`0.conditions.0.alert.name: Does not match pattern '^[a-zA-Z0-9_]{1,128}$'`,
				`0.conditions.0: Must validate at least one schema (anyOf)`,
				`0.gathering_functions: Invalid type. Expected: object, given: null`,
			},
		},
		{
			Name: "Alert name is too long",
			Rules: []GatheringRule{{
				Conditions: []ConditionWithParams{
					{
						Type: AlertIsFiring,
						Alert: &AlertConditionParams{
							Name: strings.Repeat("x", 130),
						},
					},
				},
			}},
			Errors: []string{
				`0.conditions.0.alert.name: Does not match pattern '^[a-zA-Z0-9_]{1,128}$'`,
				`0.conditions.0: Must validate at least one schema (anyOf)`,
				`0.gathering_functions: Invalid type. Expected: object, given: null`,
			},
		},
		{
			Name: "Too many conditions",
			Rules: []GatheringRule{{
				Conditions: tooManyConditions,
			}},
			Errors: []string{
				`0.conditions: Array must have at most 8 items`,
				`0.gathering_functions: Invalid type. Expected: object, given: null`,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			errs := validateGatheringRules(tc.Rules)
			assertErrsMatchStrings(t, errs, tc.Errors, tc.Name)
		})
	}
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
				`0.gathering_functions.image_streams_of_namespace.namespace: Does not match pattern '^openshift-[a-zA-Z0-9_.-]{1,128}$'`,
				`0.gathering_functions.logs_of_namespace.namespace: Does not match pattern '^openshift-[a-zA-Z0-9_.-]{1,128}$'`,
				`0.gathering_functions.logs_of_namespace.tail_lines: Must be greater than or equal to 1`,
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
		{
			Name: "GatherAPIRequestCounts invalid alert name",
			Rules: []GatheringRule{
				{
					Conditions: []ConditionWithParams{},
					GatheringFunctions: map[GatheringFunctionName]interface{}{
						GatherAPIRequestCounts: GatherAPIRequestCountsParams{
							AlertName: "??--..",
						},
					},
				},
			},
			Errors: []string{
				`0.gathering_functions.api_request_counts_of_resource_from_alert.alert_name: Does not match pattern '^[a-zA-Z0-9_]{1,128}$'`,
			},
		},
		{
			Name: "GatherAPIRequestCounts too long alert name",
			Rules: []GatheringRule{
				{
					Conditions: []ConditionWithParams{},
					GatheringFunctions: map[GatheringFunctionName]interface{}{
						GatherAPIRequestCounts: GatherAPIRequestCountsParams{
							AlertName: strings.Repeat("x", 130),
						},
					},
				},
			},
			Errors: []string{
				`0.gathering_functions.api_request_counts_of_resource_from_alert.alert_name: Does not match pattern '^[a-zA-Z0-9_]{1,128}$'`,
			},
		},
		{
			Name: "GatherImageStreamsOfNamespace invalid namespace",
			Rules: []GatheringRule{
				{
					Conditions: []ConditionWithParams{},
					GatheringFunctions: map[GatheringFunctionName]interface{}{
						GatherImageStreamsOfNamespace: GatherImageStreamsOfNamespaceParams{
							Namespace: "invalid_namespace",
						},
					},
				},
			},
			Errors: []string{
				`0.gathering_functions.image_streams_of_namespace.namespace: Does not match pattern '^openshift-[a-zA-Z0-9_.-]{1,128}$'`,
			},
		},
		{
			Name: "GatherImageStreamsOfNamespace invalid namespace 2",
			Rules: []GatheringRule{
				{
					Conditions: []ConditionWithParams{},
					GatheringFunctions: map[GatheringFunctionName]interface{}{
						GatherImageStreamsOfNamespace: GatherImageStreamsOfNamespaceParams{
							Namespace: "openshift-???",
						},
					},
				},
			},
			Errors: []string{
				`0.gathering_functions.image_streams_of_namespace.namespace: Does not match pattern '^openshift-[a-zA-Z0-9_.-]{1,128}$'`,
			},
		},
		{
			Name: "GatherImageStreamsOfNamespace too long namespace",
			Rules: []GatheringRule{
				{
					Conditions: []ConditionWithParams{},
					GatheringFunctions: map[GatheringFunctionName]interface{}{
						GatherImageStreamsOfNamespace: GatherImageStreamsOfNamespaceParams{
							Namespace: "openshift-" + strings.Repeat("x", 130),
						},
					},
				},
			},
			Errors: []string{
				`0.gathering_functions.image_streams_of_namespace.namespace: Does not match pattern '^openshift-[a-zA-Z0-9_.-]{1,128}$'`,
			},
		},
		{
			Name: "GatherContainersLogs invalid container name",
			Rules: []GatheringRule{
				{
					Conditions: []ConditionWithParams{},
					GatheringFunctions: map[GatheringFunctionName]interface{}{
						GatherContainersLogs: GatherContainersLogsParams{
							AlertName: "NonExistingAlert",
							Namespace: "openshift-namespace",
							Container: "???container",
							TailLines: 3,
							Previous:  false,
						},
					},
				},
			},
			Errors: []string{
				`0.gathering_functions.containers_logs.container: Does not match pattern '^[a-zA-Z0-9_.-]{1,128}$'`,
			},
		},
		{
			Name: "GatherContainersLogs too long container name",
			Rules: []GatheringRule{
				{
					Conditions: []ConditionWithParams{},
					GatheringFunctions: map[GatheringFunctionName]interface{}{
						GatherContainersLogs: GatherContainersLogsParams{
							AlertName: "NonExistingAlert",
							Namespace: "openshift-namespace",
							Container: strings.Repeat("x", 130),
							TailLines: 3,
							Previous:  false,
						},
					},
				},
			},
			Errors: []string{
				`0.gathering_functions.containers_logs.container: Does not match pattern '^[a-zA-Z0-9_.-]{1,128}$'`,
			},
		},
		{
			Name: "GatherContainersLogs too many tail lines",
			Rules: []GatheringRule{
				{
					Conditions: []ConditionWithParams{},
					GatheringFunctions: map[GatheringFunctionName]interface{}{
						GatherContainersLogs: GatherContainersLogsParams{
							AlertName: "NonExistingAlert",
							Namespace: "openshift-namespace",
							Container: "container",
							TailLines: 4097,
							Previous:  false,
						},
					},
				},
			},
			Errors: []string{
				`0.gathering_functions.containers_logs.tail_lines: Must be less than or equal to 4096`,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			errs := validateGatheringRules(tc.Rules)
			assertErrsMatchStrings(t, errs, tc.Errors, tc.Name)
		})
	}
}

func assertErrsMatchStrings(t *testing.T, errs []error, expectedStrings []string, message string) {
	var errStrings []string
	for _, err := range errs {
		errStrings = append(errStrings, err.Error())
	}

	assert.Equal(t, expectedStrings, errStrings, message)
}
