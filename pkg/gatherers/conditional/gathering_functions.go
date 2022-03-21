package conditional

import (
	"encoding/json"
	"fmt"
)

// To add a new gathering function, follow the next steps:
// 1. Create gather_*[_test].go file(s) with the function builder, the function and the tests
// 2. Add a value in GatheringFunctionName enum
// 3. Modify GatheringFunctionName.NewParams function to create params for this function
// 4. Create Gather*Params struct
// 5. Register the builder in gatheringFunctionBuilders map
// 6. Add validation in gathering_rule.schema.json

// GatheringFunctions is a type to map gathering function name to its params
type GatheringFunctions = map[GatheringFunctionName]interface{}

// function names:

// GatheringFunctionName defines functions of conditional gatherer
type GatheringFunctionName string

const (
	// GatherLogsOfNamespace is a function collecting logs of the provided namespace.
	// See file gather_logs_of_namespace.go
	GatherLogsOfNamespace GatheringFunctionName = "logs_of_namespace"

	// GatherImageStreamsOfNamespace is a function collecting image streams of the provided namespace.
	// See file gather_image_streams_of_namespace.go
	GatherImageStreamsOfNamespace GatheringFunctionName = "image_streams_of_namespace"

	// GatherAPIRequestCounts is a function collecting api request counts for the resources read
	// from the corresponding alert
	// See file gather_api_requests_count.go
	GatherAPIRequestCounts GatheringFunctionName = "api_request_counts_of_resource_from_alert"

	// GatherContainersLogs is a function that collects logs from pod's containers
	// See file gather_containers_logs.go
	GatherContainersLogs GatheringFunctionName = "containers_logs"

	// GatherPodDefinition is a function that collects the pod definitions
	// See file gather_pod_definition.go
	GatherPodDefinition GatheringFunctionName = "pod_definition"
)

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
	case GatherAPIRequestCounts:
		var params GatherAPIRequestCountsParams
		err := json.Unmarshal(jsonParams, &params)
		return params, err
	case GatherContainersLogs:
		var params GatherContainersLogsParams
		err := json.Unmarshal(jsonParams, &params)
		return params, err
	case GatherPodDefinition:
		var params GatherPodDefinitionParams
		err := json.Unmarshal(jsonParams, &params)
		return params, err
	}
	return nil, fmt.Errorf("unable to create params for %T: %v", name, name)
}

// params:

// GatherLogsOfNamespaceParams defines parameters for logs of namespace gatherer
type GatherLogsOfNamespaceParams struct {
	// Namespace from which to collect logs
	Namespace string `json:"namespace"`
	// A number of log lines to keep for each container
	TailLines int64 `json:"tail_lines"`
}

// GatherImageStreamsOfNamespaceParams defines parameters for image streams of namespace gatherer
type GatherImageStreamsOfNamespaceParams struct {
	// Namespace from which to collect image streams
	Namespace string `json:"namespace"`
}

// GatherAPIRequestCountsParams defines parameters for api_request_counts gatherer
type GatherAPIRequestCountsParams struct {
	AlertName string `json:"alert_name"`
}

// GatherContainersLogsParams defines parameters for container_logs gatherer
type GatherContainersLogsParams struct {
	AlertName string `json:"alert_name"`
	Namespace string `json:"namespace,omitempty"`
	Container string `json:"container,omitempty"`
	TailLines int64  `json:"tail_lines"`
	Previous  bool   `json:"previous,omitempty"`
}

// GatherPodDefinitionParams defines parameters for pod_definition gatherer
type GatherPodDefinitionParams struct {
	AlertName string `json:"alert_name"`
}

// registered builders:

// gatheringFunctionBuilders lists all the gatherers which can be run on some condition. Gatherers can have parameters,
// like namespace or number of log lines to fetch, see the docs of the functions.
var gatheringFunctionBuilders = map[GatheringFunctionName]GathererFunctionBuilderPtr{
	GatherLogsOfNamespace:         (*Gatherer).BuildGatherLogsOfNamespace,
	GatherImageStreamsOfNamespace: (*Gatherer).BuildGatherImageStreamsOfNamespace,
	GatherAPIRequestCounts:        (*Gatherer).BuildGatherAPIRequestCounts,
	GatherContainersLogs:          (*Gatherer).BuildGatherContainersLogs,
	GatherPodDefinition:           (*Gatherer).BuildGatherPodDefinition,
}
