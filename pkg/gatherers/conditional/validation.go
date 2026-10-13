package conditional

import (
	_ "embed"
	"fmt"
	"sort"

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

// validationErrors is a wrapper type for
// error used for sorting
type validationErrors []error

func (v validationErrors) Len() int {
	return len(v)
}

// Less sorts the validation errors as strings alphabetically
func (v validationErrors) Less(i, j int) bool {
	return v[i].Error() < v[j].Error()
}

func (v validationErrors) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}

// validateRemoteConfig validates the both main parts of the remote configuration.
// the original conditional gathering rules as well as the container logs
func validateRemoteConfig(remoteConfig RemoteConfiguration) []error {
	var errs validationErrors
	gatheringRulesErrs := validateGatheringRules(remoteConfig.ConditionalGatheringRules)
	errs = append(errs, gatheringRulesErrs...)
	containerLogErrs := validateContainerLogRequests(remoteConfig.ContainerLogRequests)
	errs = append(errs, containerLogErrs...)
	return errs
}

// validateGatheringRules validates provided gathering rules, will return nil on success, or a list of errors
func validateGatheringRules(gatheringRules []GatheringRule) []error {
	var errs validationErrors
	if len(gatheringRules) == 0 {
		return append(errs, fmt.Errorf("there are no conditional rules"))
	}

	if len(gatheringRulesJSONSchema) == 0 || len(gatheringRuleJSONSchema) == 0 {
		return append(errs, fmt.Errorf("unable to load JSON schemas"))
	}

	schemaLoader := gojsonschema.NewSchemaLoader()

	err := schemaLoader.AddSchemas(gojsonschema.NewStringLoader(gatheringRuleJSONSchema))
	if err != nil {
		return append(errs, err)
	}

	schema, err := schemaLoader.Compile(gojsonschema.NewStringLoader(gatheringRulesJSONSchema))
	if err != nil {
		return append(errs, err)
	}

	result, err := schema.Validate(gojsonschema.NewGoLoader(gatheringRules))
	if err != nil {
		return append(errs, err)
	}

	if !result.Valid() {
		for _, err := range result.Errors() {
			errs = append(errs, fmt.Errorf("%s", err.String()))
		}
	}
	// the json schema validation library doesn't seem to guarantee the order of the errors
	// (even though it seems to use slice for the errors) so we sort them
	sort.Sort(errs)
	return errs
}

func validateContainerLogRequests(containerLogRequests []RawLogRequest) []error {
	var errs validationErrors
	schemaLoader := gojsonschema.NewSchemaLoader()

	err := schemaLoader.AddSchemas(gojsonschema.NewStringLoader(containerLogJSONSchema))
	if err != nil {
		return append(errs, err)
	}

	schema, err := schemaLoader.Compile(gojsonschema.NewStringLoader(containerLogsJSONSchema))
	if err != nil {
		return append(errs, err)
	}

	result, err := schema.Validate(gojsonschema.NewGoLoader(containerLogRequests))
	if err != nil {
		return append(errs, err)
	}

	if !result.Valid() {
		for _, err := range result.Errors() {
			errs = append(errs, fmt.Errorf("%s", err.String()))
		}
	}
	// the json schema validation library doesn't seem to guarantee the order of the errors
	// (even though it seems to use slice for the errors) so we sort them
	sort.Sort(errs)
	return errs
}
