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

	// GatherAlertmanagerLogs is the function collection the alertmanager logs from containers
	// See file alertmanager_logs.go
	GatherAlertmanagerLogs GatheringFunctionName = "alertmanager_logs"

	// GatherLogsOfUnhealthyPods is a function collecting logs of unhealthy pods
	// See file gather_logs_of_unhealthy_pods.go
	GatherLogsOfUnhealthyPods GatheringFunctionName = "logs_of_unhealthy_pods"
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
	case GatherLogsOfUnhealthyPods:
		var result GatherLogsOfUnhealthyPodsParams
		err := json.Unmarshal(jsonParams, &result)
		return result, err
	case GatherAlertmanagerLogs:
		var params GatherAlertmanagerLogsParams
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

type GatherLogsOfUnhealthyPodsParams struct {
	AlertName string `json:"alert_name"`
	TailLines int64  `json:"tail_lines"`
	Previous  bool   `json:"previous"`
}

// GatherAlertmanagerLogsParams defines parameters for alertmanager_logs gatherer
type GatherAlertmanagerLogsParams struct {
	AlertName string `json:"alert_name"`
	TailLines int64  `json:"tail_lines"`
}
