package conditional

import (
	"encoding/json"
)

func parseGatheringRules(jsonData []byte) ([]GatheringRule, error) {
	var unmarshalledRules []GatheringRule

	err := json.Unmarshal(jsonData, &unmarshalledRules)
	if err != nil {
		return nil, err
	}

	var result []GatheringRule
	for _, unmarshalledRule := range unmarshalledRules {
		unmarshalledRule, err = convertUnmarshalledRuleToActualTypes(unmarshalledRule)
		if err != nil {
			return nil, err
		}

		result = append(result, unmarshalledRule)
	}

	return result, nil
}

func convertUnmarshalledRuleToActualTypes(rule GatheringRule) (GatheringRule, error) {
	result := GatheringRule{
		GatheringFunctions: make(GatheringFunctions),
	}

	for i := 0; i < len(rule.Conditions); i++ {
		condition := &rule.Conditions[i]

		jsonParams, err := json.Marshal(condition.Params)
		if err != nil {
			return GatheringRule{}, err
		}

		newParams, err := condition.Type.NewParams(jsonParams)
		if err != nil {
			return GatheringRule{}, err
		}

		result.Conditions = append(result.Conditions, ConditionWithParams{
			Type:   condition.Type,
			Params: newParams,
		})
	}

	for gatheringFunction, params := range rule.GatheringFunctions {
		jsonParams, err := json.Marshal(params)
		if err != nil {
			return GatheringRule{}, err
		}

		newParams, err := gatheringFunction.NewParams(jsonParams)
		if err != nil {
			return GatheringRule{}, err
		}

		result.GatheringFunctions[gatheringFunction] = newParams
	}

	return result, nil
}
