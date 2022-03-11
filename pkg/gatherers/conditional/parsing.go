package conditional

import (
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"
)

func parseGatheringRules(jsonData string) ([]GatheringRule, error) {
	var unmarshalledRules struct {
		Version string          `json:"version"`
		Rules   []GatheringRule `json:"rules"`
	}

	err := json.Unmarshal([]byte(jsonData), &unmarshalledRules)
	if err != nil {
		return nil, err
	}

	var result []GatheringRule
	for _, unmarshalledRule := range unmarshalledRules.Rules {
		unmarshalledRule.GatheringFunctions, err = parseGatheringFunctions(unmarshalledRule.GatheringFunctions)
		if err != nil {
			klog.Errorf("skipping a rule because of an error: %v %v", err, unmarshalledRule)
			continue
		}

		result = append(result, unmarshalledRule)
	}

	return result, nil
}

func parseGatheringFunctions(gatheringFunctions GatheringFunctions) (GatheringFunctions, error) {
	newGatheringFunctions := make(GatheringFunctions)
	for funcName, funcParams := range gatheringFunctions {
		funcParamsBytes, err := json.Marshal(funcParams)
		if err != nil {
			return nil, fmt.Errorf("%v", err)
		}
		newGatheringFunctions[funcName], err = funcName.NewParams(funcParamsBytes)
		if err != nil {
			return nil, fmt.Errorf("%v", err)
		}
	}

	return newGatheringFunctions, nil
}
