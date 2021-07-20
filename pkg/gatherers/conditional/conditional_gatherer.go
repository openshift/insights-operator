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
	"io/ioutil"
	"net/http"
	"sort"
	"strings"

	"github.com/prometheus/common/expfmt"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
)

const gatheringRulesEndpoint = "http://localhost:8080/rules.json"

// gatheringFunctionBuilders lists all the gatherers which can be run on some condition. Gatherers can have parameters,
// like namespace or number of log lines to fetch, see the docs of the functions.
var gatheringFunctionBuilders = map[GatheringFunctionName]GathererFunctionBuilderPtr{
	GatherLogsOfNamespace:         (*Gatherer).BuildGatherLogsOfNamespace,
	GatherImageStreamsOfNamespace: (*Gatherer).BuildGatherImageStreamsOfNamespace,
}

const canConditionalGathererFail = false

// Gatherer implements the conditional gatherer
type Gatherer struct {
	gatherProtoKubeConfig   *rest.Config
	metricsGatherKubeConfig *rest.Config
	imageKubeConfig         *rest.Config
	firingAlerts            map[string]bool // golang doesn't have sets :(
	gatheringRulesEndpoint  string
	gatheringRules          []GatheringRule
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
		gatheringRulesEndpoint:  gatheringRulesEndpoint,
	}
}

// GetName returns the name of the gatherer
func (g *Gatherer) GetName() string {
	return "conditional"
}

// GetGatheringFunctions returns gathering functions that should be run considering the conditions
// + the gathering function producing metadata for the conditional gatherer
func (g *Gatherer) GetGatheringFunctions(ctx context.Context) (map[string]gatherers.GatheringClosure, error) {
	gatheringRulesJSON, err := g.getGatheringRulesJSON()
	if err != nil {
		klog.Errorf("unable to load gathering rules: %v", err)
		return nil, nil
	}

	g.gatheringRules, err = parseGatheringRules(gatheringRulesJSON)
	if err != nil {
		klog.Errorf("unable to parse gathering rules: %v", err)
		return nil, nil
	}

	// later the config will be downloaded from an external source
	errs := validateGatheringRules(g.gatheringRules)
	if len(errs) > 0 {
		klog.Errorf("got invalid config for conditional gatherer: %v", utils.SumErrors(errs))
		return nil, nil
	}

	err = g.updateAlertsCache(ctx)
	if err != nil {
		klog.Errorf("conditional gatherer can't update alerts cache: %v", err)
		return nil, nil
	}

	gatheringFunctions := make(map[string]gatherers.GatheringClosure)

	gatheringFunctions["conditional_gatherer_rules"] = gatherers.GatheringClosure{
		Run:     g.GatherConditionalGathererRules,
		CanFail: canConditionalGathererFail,
	}

	for _, conditionalGathering := range g.gatheringRules {
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
			Item: record.JSONMarshaller{Object: g.gatheringRules},
		},
	}, nil
}

// areAllConditionsSatisfied returns true if all the conditions are satisfied, for example if the condition is
// to check if a metric is firing, it will look at that metric and return the result according to that
func (g *Gatherer) areAllConditionsSatisfied(conditions []ConditionWithParams) (bool, error) {
	for _, condition := range conditions {
		switch condition.Type {
		case AlertIsFiring:
			params, ok := condition.Params.(AlertIsFiringConditionParams)
			if !ok {
				return false, fmt.Errorf(
					"invalid params type, expected %T, got %T",
					AlertIsFiringConditionParams{}, condition.Params,
				)
			}

			if !g.isAlertFiring(params.Name) {
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
	gatheringFunctions map[GatheringFunctionName]interface{},
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

func (g *Gatherer) getGatheringRulesJSON() ([]byte, error) {
	resp, err := http.Get(g.gatheringRulesEndpoint)
	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(resp.Body)
}

// getConditionalGatheringFunctionName creates a name of the conditional gathering function adding the parameters
// after the name. For example:
//   "conditional/logs_of_namespace/namespace=openshift-cluster-samples-operator,tail_lines=100"
func getConditionalGatheringFunctionName(funcName string, gatherParamsInterface interface{}) string {
	gatherParams, err := utils.StructToMap(gatherParamsInterface)
	if err != nil {
		// will only happen when non struct is passed which means code is completely broken and panicking is ok
		panic(err)
	}

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
