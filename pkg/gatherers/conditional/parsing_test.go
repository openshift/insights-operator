package conditional

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ParseConditionalGathererConfig(t *testing.T) {
	config, err := parseGatheringRules([]byte("[]"))
	assert.NoError(t, err)
	assert.Empty(t, config)

	config, err = parseGatheringRules([]byte("[{}]"))
	assert.NoError(t, err)
	assert.Len(t, config, 1)
	assert.Nil(t, config[0].Conditions)
	assert.Empty(t, config[0].GatheringFunctions)

	// an invalid config should be unmarshalled
	config, err = parseGatheringRules([]byte(`
		[
			{
				"conditions": [
					{
						"type": "alert_is_firing",
						"params": { "name": "TestAlert" }
					},
					{
						"type": "alert_is_firing",
						"params": { "name": "invalid alert name" }
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
	`))
	assert.NoError(t, err)
	assert.Len(t, config, 2)
	assert.Len(t, config[0].Conditions, 2)
	assert.Len(t, config[0].GatheringFunctions, 2)
	assert.ElementsMatch(t, []ConditionWithParams{
		{
			Type:   AlertIsFiring,
			Params: AlertIsFiringConditionParams{Name: "TestAlert"},
		},
		{
			Type:   AlertIsFiring,
			Params: AlertIsFiringConditionParams{Name: "invalid alert name"},
		},
	}, config[0].Conditions)
	assert.Equal(t, GatheringFunctions{
		GatherLogsOfNamespace: GatherLogsOfNamespaceParams{
			Namespace: "openshift-something",
			TailLines: 128,
		},
		GatherImageStreamsOfNamespace: GatherImageStreamsOfNamespaceParams{
			Namespace: "invalid param",
		},
	}, config[0].GatheringFunctions)

	assert.Nil(t, config[1].Conditions)
	assert.Empty(t, config[1].GatheringFunctions)

	// but validation of an invalid config should fail

	errs := validateGatheringRules(config)
	assert.NotEmpty(t, errs)

	// test the valid config
	config, err = parseGatheringRules([]byte(`
		[
			{
				"conditions": [
					{
						"type": "alert_is_firing",
						"params": { "name": "TestAlert" }
					},
					{
						"type": "alert_is_firing",
						"params": { "name": "TestAlert1" }
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
	`))
	assert.NoError(t, err)

	errs = validateGatheringRules(config)
	assert.Empty(t, errs)
}

func Test_ParseConditionalGathererConfig_Error(t *testing.T) {
	config, err := parseGatheringRules([]byte("["))
	assert.Nil(t, config)
	assert.EqualError(t, err, "unexpected end of JSON input")

	config, err = parseGatheringRules([]byte(`
		[
			{
				"conditions": [
					{
						"type": "invalid alert name",
						"params": { "name": "TestAlert" }
					}
				]
			}
		]
	`))
	assert.EqualError(t, err, "unable to create params for conditional.ConditionType: invalid alert name")

	config, err = parseGatheringRules([]byte(`
		[
			{
				"gathering_functions": {
					"invalid gathering function name": {
						"namespace": "openshift-something",
						"tail_lines": 128
					}
				}
			}
		]
	`))
	assert.EqualError(
		t,
		err,
		"unable to create params for conditional.GatheringFunctionName: invalid gathering function name",
	)
}
