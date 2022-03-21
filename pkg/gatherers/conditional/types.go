package conditional

import (
	"github.com/openshift/insights-operator/pkg/gatherers"
)

// GatheringRule is a rule consisting of conditions and gathering functions to run if all conditions are met,
// gathering_rule.schema.json describes valid values for this struct.
// An example of it:
// {
//   "conditions": [
//     {
//       "type": "alert_is_firing",
//       "alert": {
//         "name": "ClusterVersionOperatorIsDown"
//       }
//     },
//     {
//       "type": "cluster_version_matches",
//       "cluster_version": {
//         "version": "4.8.x"
//       }
//     }
//   ],
//   "gathering_functions": {
//     "gather_logs_of_namespace": {
//       "namespace": "openshift-cluster-version",
//       "keep_lines": 100
//     }
//   }
// }
// Which means to collect logs of all containers from all pods in namespace openshift-monitoring keeping last 100 lines
// per container only if cluster version is 4.8 (not implemented, just an example of possible use) and alert
// ClusterVersionOperatorIsDown is firing
type GatheringRule struct {
	// conditions can be empty
	Conditions []ConditionWithParams `json:"conditions"`
	// gathering functions can't be empty
	GatheringFunctions GatheringFunctions `json:"gathering_functions"`
}

// GathererFunctionBuilderPtr defines a pointer to a gatherer function builder
type GathererFunctionBuilderPtr = func(*Gatherer, interface{}) (gatherers.GatheringClosure, error)

// AlertLabels defines alert labels as a string key/value pairs
type AlertLabels map[string]string
