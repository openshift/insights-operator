package conditional

import (
	"regexp"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	InvalidReason      = "Invalid"
	SucceededReason    = "Succeeded"
	NotAvailableReason = "NotAvailable"
)

// RemoteConfiguration is a structure to hold gathering rules with their version
type RemoteConfiguration struct {
	Version                   string          `json:"version"`
	ConditionalGatheringRules []GatheringRule `json:"conditional_gathering_rules"`
	ContainerLogRequests      []RawLogRequest `json:"container_logs"`
}

// GatheringRule is a rule consisting of conditions and gathering functions to run if all conditions are met,
// gathering_rule.schema.json describes valid values for this struct.
// An example of it:
//
//	{
//	  "conditions": [
//	    {
//	      "type": "alert_is_firing",
//	      "alert": {
//	        "name": "ClusterVersionOperatorIsDown"
//	      }
//	    },
//	    {
//	      "type": "cluster_version_matches",
//	      "cluster_version": {
//	        "version": "4.8.x"
//	      }
//	    }
//	  ],
//	  "gathering_functions": {
//	    "gather_logs_of_namespace": {
//	      "namespace": "openshift-cluster-version",
//	      "keep_lines": 100
//	    }
//	  }
//	}
//
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

// RawLogRequest is type used to unmarshal the remote
// configuration JSON
type RawLogRequest struct {
	Namespace    string   `json:"namespace"`
	PodNameRegex string   `json:"pod_name_regex"`
	Messages     []string `json:"messages"`
	Previous     bool     `json:"previous,omitempty"`
}

// LogRequest is a "sanitized" type, because
// there can be various requests for the same namespace and this type
// helps prevent duplicate Pod name regular expressions and duplicate messages
type LogRequest struct {
	Namespace              string
	PodNameRegexToMessages map[PodNameRegexPrevious]sets.Set[string]
}

// PodNameRegexPrevious is a helper struct storing
// the Pod name regular expression value together with
// a flag saying whether it is for previous container log or not
type PodNameRegexPrevious struct {
	PodNameRegex string
	Previous     bool
}

// ContainerLogRequest is a type representing concrete and unique
// container log request
type ContainerLogRequest struct {
	Namespace     string
	PodName       string
	ContainerName string
	Previous      bool
	MessageRegex  *regexp.Regexp
}
