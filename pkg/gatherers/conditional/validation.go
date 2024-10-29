package conditional

import (
	_ "embed"
	"fmt"

	"github.com/xeipuuv/gojsonschema"
)

//go:embed gathering_rule.schema.json
var gatheringRuleJSONSchema string

//go:embed gathering_rules.schema.json
var gatheringRulesJSONSchema string

//go:embed container_log.schema.json
var containerLogJSONSchema string

//go:embed container_logs.schema.json
var containerLogsJSONSchema string

// validateRemoteConfig validates the both main parts of the remote configuration.
// the original conditional gathering rules as well as the container logs
func validateRemoteConfig(remoteConfig RemoteConfiguration) []error {
	var errs []error
	gatheringRulesErrs := validateGatheringRules(remoteConfig.ConditionalGatheringRules)
	errs = append(errs, gatheringRulesErrs...)
	containerLogErrs := validateContainerLogRequests(remoteConfig.ContainerLogRequests)
	errs = append(errs, containerLogErrs...)
	return errs
}

// validateGatheringRules validates provided gathering rules, will return nil on success, or a list of errors
func validateGatheringRules(gatheringRules []GatheringRule) []error {
	if len(gatheringRules) == 0 {
		return []error{fmt.Errorf("there are no conditional rules")}
	}

	if len(gatheringRulesJSONSchema) == 0 || len(gatheringRuleJSONSchema) == 0 {
		return []error{fmt.Errorf("unable to load JSON schemas")}
	}

	schemaLoader := gojsonschema.NewSchemaLoader()

	err := schemaLoader.AddSchemas(gojsonschema.NewStringLoader(gatheringRuleJSONSchema))
	if err != nil {
		return []error{err}
	}

	schema, err := schemaLoader.Compile(gojsonschema.NewStringLoader(gatheringRulesJSONSchema))
	if err != nil {
		return []error{err}
	}

	result, err := schema.Validate(gojsonschema.NewGoLoader(gatheringRules))
	if err != nil {
		return []error{err}
	}

	if !result.Valid() {
		var errs []error
		for _, err := range result.Errors() {
			errs = append(errs, fmt.Errorf("%s", err.String()))
		}

		return errs
	}

	return nil
}

func validateContainerLogRequests(containerLogRequests []RawLogRequest) []error {
	schemaLoader := gojsonschema.NewSchemaLoader()

	err := schemaLoader.AddSchemas(gojsonschema.NewStringLoader(containerLogJSONSchema))
	if err != nil {
		return []error{err}
	}

	schema, err := schemaLoader.Compile(gojsonschema.NewStringLoader(containerLogsJSONSchema))
	if err != nil {
		return []error{err}
	}

	result, err := schema.Validate(gojsonschema.NewGoLoader(containerLogRequests))
	if err != nil {
		return []error{err}
	}

	if !result.Valid() {
		var errs []error
		for _, err := range result.Errors() {
			errs = append(errs, fmt.Errorf("%s", err.String()))
		}
		return errs
	}

	return nil
}
