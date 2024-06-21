package conditional

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ParseConditionalGathererConfig(t *testing.T) { //nolint:funlen
	config, err := parseRemoteConfiguration([]byte("{}"))
	assert.NoError(t, err)
	assert.Empty(t, config)

	config, err = parseRemoteConfiguration([]byte(`{"conditional_gathering_rules": [{}]}`))
	assert.NoError(t, err)
	assert.NotNil(t, config)

	rules := config.ConditionalGatheringRules
	assert.Len(t, rules, 1)
	assert.Nil(t, rules[0].Conditions)
	assert.Empty(t, rules[0].GatheringFunctions)

	// an invalid config should be unmarshalled
	config, err = parseRemoteConfiguration([]byte(`{
		"version": "1.0.0",
		"conditional_gathering_rules": [
			{
				"conditions": [
					{
						"type": "alert_is_firing",
						"alert": { "name": "TestAlert" }
					},
					{
						"type": "alert_is_firing",
						"alert": { "name": "invalid alert name" }
					}
				],
				"gathering_functions": {
					"logs_of_namespace": {
						"namespace": "openshift-something",
						"tail_lines": 128
					},
					"image_streams_of_namespace": { "namespace": "invalid param" }
				}
			},
			{}
		]
	}`))
	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, "1.0.0", config.Version)

	rules = config.ConditionalGatheringRules
	assert.Len(t, rules, 2)
	assert.Len(t, rules[0].Conditions, 2)
	assert.Len(t, rules[0].GatheringFunctions, 2)
	assert.ElementsMatch(t, []ConditionWithParams{
		{
			Type:  AlertIsFiring,
			Alert: &AlertConditionParams{Name: "TestAlert"},
		},
		{
			Type:  AlertIsFiring,
			Alert: &AlertConditionParams{Name: "invalid alert name"},
		},
	}, rules[0].Conditions)
	assert.Equal(t, GatheringFunctions{
		GatherLogsOfNamespace: GatherLogsOfNamespaceParams{
			Namespace: "openshift-something",
			TailLines: 128,
		},
		GatherImageStreamsOfNamespace: GatherImageStreamsOfNamespaceParams{
			Namespace: "invalid param",
		},
	}, rules[0].GatheringFunctions)

	assert.Nil(t, rules[1].Conditions)
	assert.Empty(t, rules[1].GatheringFunctions)

	// but validation of an invalid config should fail

	errs := validateGatheringRules(rules)
	assert.NotEmpty(t, errs)

	// test the valid config
	config, err = parseRemoteConfiguration([]byte(`{
		"version": "1.0.0",
		"conditional_gathering_rules": [
			{
				"conditions": [
					{
						"type": "alert_is_firing",
						"alert": { "name": "TestAlert" }
					},
					{
						"type": "alert_is_firing",
						"alert": { "name": "TestAlert1" }
					}
				],
				"gathering_functions": {
					"logs_of_namespace": {
						"namespace": "openshift-something",
						"tail_lines": 128
					},
					"image_streams_of_namespace": { "namespace": "openshift-related-namespace" }
				}
			}
		]
	}`))
	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, "1.0.0", config.Version)

	errs = validateGatheringRules(config.ConditionalGatheringRules)
	assert.Empty(t, errs)
}
