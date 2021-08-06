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

// validateGatheringRules validates provided gathering rules, will return nil on success, or a list of errors
func validateGatheringRules(gatheringRules []GatheringRule) []error {
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
			errs = append(errs, fmt.Errorf(err.String()))
		}

		return errs
	}

	return nil
}
