package conditional

import (
	"encoding/json"
	"fmt"
)

// GatheringFunctions is a type to map gathering function name to its params
type GatheringFunctions = map[GatheringFunctionName]interface{}

// function names:

// GatheringFunctionName defines functions of conditional gatherer
type GatheringFunctionName string

// GatherLogsOfNamespace is a function collecting logs of the provided namespace.
// See file gather_logs_of_namespace.go
const GatherLogsOfNamespace GatheringFunctionName = "logs_of_namespace"

// GatherImageStreamsOfNamespace is a function collecting image streams of the provided namespace.
// See file gather_image_streams_of_namespace.go
const GatherImageStreamsOfNamespace GatheringFunctionName = "image_streams_of_namespace"

// IsValid checks if the value is one of allowed for this "enum"
func (name GatheringFunctionName) IsValid() error {
	switch name {
	case GatherLogsOfNamespace, GatherImageStreamsOfNamespace:
		return nil
	}
	return fmt.Errorf("invalid value for %T: %v", name, name)
}

func (name GatheringFunctionName) NewParams(jsonParams []byte) (interface{}, error) {
	switch name {
	case GatherLogsOfNamespace:
		var result GatherLogsOfNamespaceParams
		err := json.Unmarshal(jsonParams, &result)
		return result, err
	case GatherImageStreamsOfNamespace:
		var result GatherImageStreamsOfNamespaceParams
		err := json.Unmarshal(jsonParams, &result)
		return result, err
	}
	return nil, fmt.Errorf("unable to create params for %T: %v", name, name)
}

// params:

// GatherLogsOfNamespaceParams defines parameters for logs of namespace gatherer
type GatherLogsOfNamespaceParams struct {
	// Namespace from which to collect logs. Should be a valid openshift namespace (see validation.go)
	Namespace string `json:"namespace" validate:"openshift_namespace"`
	// A number of log lines to keep for each container. Should be in range from 1 to 4096 (including)
	TailLines int64 `json:"tail_lines" validate:"min=1,max=4096"`
}

// GatherImageStreamsOfNamespaceParams defines parameters for image streams of namespace gatherer
type GatherImageStreamsOfNamespaceParams struct {
	// Namespace from which to collect image streams
	Namespace string `json:"namespace" validate:"openshift_namespace"`
}

// GatheringFunctionNameToParamsType maps GatheringFunctionName to Params, needed for validation,
// you gotta add a new value here whenever you implement a new gathering function
var GatheringFunctionNameToParamsType = map[GatheringFunctionName]interface{}{
	GatherLogsOfNamespace:         GatherLogsOfNamespaceParams{},
	GatherImageStreamsOfNamespace: GatherImageStreamsOfNamespaceParams{},
}
