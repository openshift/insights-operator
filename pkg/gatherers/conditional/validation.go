package conditional

import (
	"fmt"
	"reflect"

	"github.com/go-playground/validator"
)

var validate *validator.Validate

func init() {
	validate = validator.New()

	validate.RegisterStructValidation(gatheringRuleValidator, GatheringRule{})
	validate.RegisterAlias("openshift_namespace", "min=1,max=128,hostname,startswith=openshift-")
}

func validateGatheringRules(gatheringRules []GatheringRule) []error {
	var allErrors []error

	for _, gatheringRule := range gatheringRules {
		errs := validateGatheringFunctionsParamsAreStructs(gatheringRule.GatheringFunctions)
		if len(errs) > 0 {
			allErrors = append(allErrors, errs...)
			continue
		}

		err := validate.Struct(gatheringRule)
		if err != nil {
			allErrors = append(allErrors, err)
			continue
		}
	}

	return allErrors
}

func validateGatheringFunctionsParamsAreStructs(gatheringFunctions GatheringFunctions) (errs []error) {
	// a workaround to not panic https://github.com/go-playground/validator/issues/789
	for key, val := range gatheringFunctions {
		if reflect.TypeOf(val).Kind() != reflect.Struct {
			errs = append(errs, fmt.Errorf("gathering function params should be a struct, key is %v", key))
		}
	}
	return
}

func gatheringRuleValidator(sl validator.StructLevel) {
	gatheringRule := sl.Current().Interface().(GatheringRule)

	for i := range gatheringRule.Conditions {
		validateCondition(&gatheringRule.Conditions[i], sl)
	}

	if len(gatheringRule.GatheringFunctions) == 0 {
		sl.ReportError(gatheringRule.GatheringFunctions, "GatheringFunctions", "GatheringFunctions", "not_empty", "")
	}

	for name, params := range gatheringRule.GatheringFunctions {
		validateGatheringFunction(name, params, sl)
	}
}

// validateCondition validates that a value of type ConditionWithParams is valid.
// Valid value should have a valid Type and Params of the corresponding type
func validateCondition(condition *ConditionWithParams, sl validator.StructLevel) {
	if err := condition.Type.IsValid(); err != nil {
		sl.ReportError(
			condition.Type,
			"Conditions[].Type",
			"Conditions[].Type",
			"is_valid",
			err.Error(),
		)
	} else {
		if reflect.TypeOf(condition.Params) != reflect.TypeOf(ConditionTypeToParamsType[condition.Type]) {
			sl.ReportError(
				condition.Params,
				"Conditions[].Params",
				"Conditions[].Params",
				"is_valid_type",
				fmt.Sprintf("params cannot be %T for %T", condition.Params, condition.Type),
			)
		}
	}
}

// validateGatheringFunction validates that a value of type GatheringFunctions is valid.
// Valid value should have valid function names and corresponding params type
func validateGatheringFunction(name GatheringFunctionName, params interface{}, sl validator.StructLevel) {
	if err := name.IsValid(); err != nil {
		sl.ReportError(
			name,
			"GatheringFunctions[].Name",
			"GatheringFunctions[].Name",
			"is_valid",
			err.Error(),
		)
	} else {
		if reflect.TypeOf(params) != reflect.TypeOf(GatheringFunctionNameToParamsType[name]) {
			sl.ReportError(
				params,
				"GatheringFunctions[].Params",
				"GatheringFunctions[].Params",
				"is_valid_type",
				fmt.Sprintf("params cannot be %T for %T", params, name),
			)
		}
	}
}
