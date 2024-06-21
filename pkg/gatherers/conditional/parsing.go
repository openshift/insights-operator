package conditional

import (
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"
)

func parseRemoteConfiguration(data []byte) (RemoteConfiguration, error) {
	var remoteConfig RemoteConfiguration

	err := json.Unmarshal(data, &remoteConfig)
	if err != nil {
		return RemoteConfiguration{}, err
	}

	var result []GatheringRule
	for _, unmarshalledRule := range remoteConfig.ConditionalGatheringRules {
		unmarshalledRule.GatheringFunctions, err = parseGatheringFunctions(unmarshalledRule.GatheringFunctions)
		if err != nil {
			klog.Errorf("skipping a rule because of an error: %v %v", err, unmarshalledRule)
			continue
		}

		result = append(result, unmarshalledRule)
	}

	// changing to correctly parsed rules
	remoteConfig.ConditionalGatheringRules = result

	return remoteConfig, nil
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
