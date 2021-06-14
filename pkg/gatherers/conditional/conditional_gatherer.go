// conditional package provides conditional gatherer which runs gatherings based on the rules and only if the provided
// conditions are satisfied. Right now the rules are in the code (see `gatheringRules` at line 32), but later
// they can be fetched from outside, checked that they make sense (we want to check the parameters, for example if
// a rule tells to collect logs of a namespace on firing alert, we want to check that the namespace is created
// by openshift and not by a user). Conditional gathering isn't considered prioritized, so we run it every 6 hours.
package conditional

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/prometheus/common/expfmt"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"
)

// gatheringFunctionBuilders lists all the gatherers which can be run on some condition. Gatherers can have parameters,
// like namespace or number of log lines to fetch, see the docs of the functions.
var gatheringFunctionBuilders = map[gatheringFunctionName]gathererFunctionBuilderPtr{
	gatherLogsOfNamespace:         (*Gatherer).BuildGatherLogsOfNamespace,
	gatherImageStreamsOfNamespace: (*Gatherer).BuildGatherImageStreamsOfNamespace,
}

// gatheringRules contains all the rules used to run conditional gatherings.
// Right now they are declared here in the code, but later they can be fetched from an external source.
// An example of the future json version of this is:
//   [{
//     "conditions": [
//       {
//         "type": "alert_is_firing",
//         "params": {
//           "name": "ClusterVersionOperatorIsDown"
//         }
//       },
//       {
//         "type": "cluster_version_equals",
//         "params": {
//           "version": "4.8"
//         }
//       }
//     ],
//     "gathering_functions": {
//       "gather_logs_of_namespace": {
//         "namespace": "openshift-cluster-version",
//         "keep_lines": 100
//       },
//     }
//   }]
// Which means to collect logs of all containers from all pods in namespace openshift-monitoring keeping last 100 lines
// per container only if cluster version is 4.8 (not implemented, just an example of possible use) and alert
// ClusterVersionOperatorIsDown is firing
var gatheringRules = []gatheringRule{
	{
		Conditions: []conditionWithParams{
			{
				Type: alertIsFiring,
				Params: map[string]interface{}{
					"name": "SamplesImagestreamImportFailing",
				},
			},
		},
		GatheringFunctions: map[gatheringFunctionName]GatheringFunctionParams{
			gatherLogsOfNamespace: map[string]interface{}{
				"namespace":      "openshift-cluster-samples-operator",
				"tail_lines":     100,
				"label_selector": "",
			},
			gatherImageStreamsOfNamespace: map[string]interface{}{
				"namespace": "openshift-cluster-samples-operator",
			},
		},
	},
}

const canConditionalGathererFail = false

// Gatherer implements the conditional gatherer
type Gatherer struct {
	gatherProtoKubeConfig   *rest.Config
	metricsGatherKubeConfig *rest.Config
	imageKubeConfig         *rest.Config
	firingAlerts            map[string]bool // golang doesn't have sets :(
}

// New creates a new instance of conditional gatherer with the appropriate configs
func New(gatherProtoKubeConfig, metricsGatherKubeConfig *rest.Config) *Gatherer {
	imageKubeConfig := rest.CopyConfig(gatherProtoKubeConfig)
	imageKubeConfig.QPS = common.ImageConfigQPS
	imageKubeConfig.Burst = common.ImageConfigBurst

	return &Gatherer{
		gatherProtoKubeConfig:   gatherProtoKubeConfig,
		metricsGatherKubeConfig: metricsGatherKubeConfig,
		imageKubeConfig:         imageKubeConfig,
	}
}

// GetName returns the name of the gatherer
func (g *Gatherer) GetName() string {
	return "conditional"
}

// GetGatheringFunctions returns gathering functions that should be run considering the conditions
// + the gathering function producing metadata for the conditional gatherer
func (g *Gatherer) GetGatheringFunctions(ctx context.Context) (map[string]gatherers.GatheringClosure, error) {
	err := g.updateAlertsCache(ctx)
	if err != nil {
		klog.Errorf("conditional gatherer can't update alerts cache")
		return nil, nil
	}

	gatheringFunctions := make(map[string]gatherers.GatheringClosure)

	gatheringFunctions["conditional_gatherer_rules"] = gatherers.GatheringClosure{
		Run:     g.GatherConditionalGathererRules,
		CanFail: canConditionalGathererFail,
	}

	for _, conditionalGathering := range gatheringRules {
		allConditionsAreSatisfied, err := g.areAllConditionsSatisfied(conditionalGathering.Conditions)
		if err != nil {
			return nil, err
		}

		if allConditionsAreSatisfied {
			functions, errs := g.createGatheringClosures(conditionalGathering.GatheringFunctions)
			if len(errs) > 0 {
				return nil, err
			}

			for funcName, function := range functions {
				gatheringFunctions[funcName] = function
			}
		}
	}

	return gatheringFunctions, nil
}

// GatherConditionalGathererRules stores the gathering rules in insights-operator/conditional-gatherer-rules.json
func (g *Gatherer) GatherConditionalGathererRules(context.Context) ([]record.Record, []error) {
	return []record.Record{
		{
			Name: "insights-operator/conditional-gatherer-rules",
			Item: record.JSONMarshaller{Object: gatheringRules},
		},
	}, nil
}

// areAllConditionsSatisfied returns true if all the conditions are satisfied, for example if the condition is
// to check if a metric is firing, it will look at that metric and return the result according to that
func (g *Gatherer) areAllConditionsSatisfied(conditions []conditionWithParams) (bool, error) {
	for _, condition := range conditions {
		switch condition.Type {
		case alertIsFiring:
			alertName, err := getStringFromMap(condition.Params, "name")
			if err != nil {
				return false, err
			}

			if !g.isAlertFiring(alertName) {
				return false, nil
			}
		default:
			return false, fmt.Errorf("unknown condition type: %v", condition.Type)
		}
	}

	return true, nil
}

// updateAlertsCache updates the cache with firing alerts
func (g *Gatherer) updateAlertsCache(ctx context.Context) error {
	if g.metricsGatherKubeConfig == nil {
		return nil
	}

	metricsClient, err := rest.RESTClientFor(g.metricsGatherKubeConfig)
	if err != nil {
		return err
	}

	return g.updateAlertsCacheFromClient(ctx, metricsClient)
}

func (g *Gatherer) updateAlertsCacheFromClient(ctx context.Context, metricsClient rest.Interface) error {
	const logPrefix = "conditional gatherer: "

	g.firingAlerts = make(map[string]bool)

	data, err := metricsClient.Get().AbsPath("federate").
		Param("match[]", `ALERTS{alertstate="firing"}`).
		DoRaw(ctx)
	if err != nil {
		return err
	}

	var parser expfmt.TextParser
	metricFamilies, err := parser.TextToMetricFamilies(bytes.NewReader(data))
	if err != nil {
		return err
	}

	if len(metricFamilies) > 1 {
		// just log cuz everything would still work
		klog.Warning(logPrefix + "unexpected output from prometheus metrics parser")
	}

	metricFamily, found := metricFamilies["ALERTS"]
	if !found {
		klog.Info(logPrefix + "no alerts are firing")
		return nil
	}

	for _, metric := range metricFamily.GetMetric() {
		if metric == nil {
			klog.Info(logPrefix + "metric is nil")
			continue
		}

		for _, label := range metric.GetLabel() {
			if label == nil {
				klog.Info(logPrefix + "label is nil")
				continue
			}

			if label.GetName() == "alertname" {
				g.firingAlerts[label.GetValue()] = true
			}
		}
	}

	return nil
}

// isAlertFiring using the cache it returns true if the alert is firing
func (g *Gatherer) isAlertFiring(alertName string) bool {
	_, alertIsFiring := g.firingAlerts[alertName]
	return alertIsFiring
}

// createGatheringClosures produces gathering closures from the rules
func (g *Gatherer) createGatheringClosures(
	gatheringFunctions map[gatheringFunctionName]GatheringFunctionParams,
) (map[string]gatherers.GatheringClosure, []error) {
	resultingClosures := make(map[string]gatherers.GatheringClosure)
	var errs []error

	for functionName, functionParams := range gatheringFunctions {
		builderFunc, found := gatheringFunctionBuilders[functionName]
		if !found {
			errs = append(errs, fmt.Errorf("unknown action type: %v", functionName))
			continue
		}

		closure, err := builderFunc(g, functionParams)
		if err != nil {
			errs = append(errs, err)
		} else {
			name := getConditionalGatheringFunctionName(string(functionName), functionParams)
			resultingClosures[name] = closure
		}
	}

	return resultingClosures, errs
}

// getConditionalGatheringFunctionName creates a name of the conditional gathering function adding the parameters
// after the name. For example:
//   "conditional/logs_of_namespace/namespace=openshift-cluster-samples-operator,tail_lines=100"
func getConditionalGatheringFunctionName(funcName string, gatherParams map[string]interface{}) string {
	if len(gatherParams) > 0 {
		funcName += "/"
	}

	type Param struct {
		name  string
		value string
	}
	var params []Param
	for key, val := range gatherParams {
		val := fmt.Sprintf("%v", val)
		if len(val) > 0 {
			params = append(params, Param{
				name:  key,
				value: val,
			})
		}
	}

	sort.Slice(params, func(i, j int) bool {
		return params[i].name < params[j].name
	})

	for _, param := range params {
		funcName += fmt.Sprintf("%v=%v,", param.name, param.value)
	}

	funcName = strings.TrimSuffix(funcName, ",")

	return funcName
}

func getInterfaceFromMap(m map[string]interface{}, key string) (interface{}, error) {
	val, found := m[key]
	if !found {
		return nil, fmt.Errorf("unable to find a value with key '%v' in the map '%v'", key, m)
	}

	return val, nil
}

func getStringFromMap(m map[string]interface{}, key string) (string, error) {
	val, err := getInterfaceFromMap(m, key)
	if err != nil {
		return "", err
	}

	res, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("unable to convert '%v' to string", val)
	}

	return res, nil
}

func getInt64FromMap(m map[string]interface{}, key string) (int64, error) {
	val, err := getInterfaceFromMap(m, key)
	if err != nil {
		return 0, err
	}

	res64, ok := val.(int64)
	if ok {
		return res64, nil
	}

	res, ok := val.(int)
	if ok {
		return int64(res), nil
	}

	resStr, err := getStringFromMap(m, key)
	if err != nil {
		return 0, err
	}

	return strconv.ParseInt(resStr, 10, 64)
}

func getPositiveInt64FromMap(m map[string]interface{}, key string) (int64, error) {
	res, err := getInt64FromMap(m, key)
	if err != nil {
		return 0, err
	}

	if res < 0 {
		return 0, fmt.Errorf("positive int expected, got '%v'", res)
	}

	return res, nil
}
