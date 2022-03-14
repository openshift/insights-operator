// conditional package provides conditional gatherer which runs gatherings based on the rules and only if the provided
// conditions are satisfied. Right now the rules are in the code (see `gatheringRules` at line 32), but later
// they can be fetched from outside, checked that they make sense (we want to check the parameters, for example if
// a rule tells to collect logs of a namespace on firing alert, we want to check that the namespace is created
// by openshift and not by a user). Conditional gathering isn't considered prioritized, so we run it every 6 hours.
//
// To add a new condition, follow the next steps:
// 1. Add structures to conditions.go
// 2. Change areAllConditionsSatisfied function in conditional_gatherer.go
package conditional

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/blang/semver/v4"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/prometheus/common/expfmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
)

// gatheringFunctionBuilders lists all the gatherers which can be run on some condition. Gatherers can have parameters,
// like namespace or number of log lines to fetch, see the docs of the functions.
var gatheringFunctionBuilders = map[GatheringFunctionName]GathererFunctionBuilderPtr{
	GatherLogsOfNamespace:         (*Gatherer).BuildGatherLogsOfNamespace,
	GatherImageStreamsOfNamespace: (*Gatherer).BuildGatherImageStreamsOfNamespace,
	GatherAPIRequestCounts:        (*Gatherer).BuildGatherAPIRequestCounts,
	GatherContainersLogs:          (*Gatherer).BuildGatherContainersLogs,
	GatherPodDefinition:           (*Gatherer).BuildGatherPodDefinition,
}

// Gatherer implements the conditional gatherer
type Gatherer struct {
	gatherProtoKubeConfig   *rest.Config
	metricsGatherKubeConfig *rest.Config
	imageKubeConfig         *rest.Config
	gatherKubeConfig        *rest.Config
	// there can be multiple instances of the same alert
	firingAlerts                map[string][]AlertLabels
	gatheringRules              []GatheringRule
	clusterVersion              string
	configurator                configobserver.Configurator
	gatheringRulesServiceClient GatheringRulesServiceClient
}

type GatheringRulesServiceClient interface {
	RecvGatheringRules(ctx context.Context, endpoint string) ([]byte, error)
}

// New creates a new instance of conditional gatherer with the appropriate configs
func New(
	gatherProtoKubeConfig, metricsGatherKubeConfig, gatherKubeConfig *rest.Config,
	configurator configobserver.Configurator, gatheringRulesServiceClient GatheringRulesServiceClient,
) *Gatherer {
	var imageKubeConfig *rest.Config
	if gatherProtoKubeConfig != nil {
		// needed for getting image streams
		imageKubeConfig = rest.CopyConfig(gatherProtoKubeConfig)
		imageKubeConfig.QPS = common.ImageConfigQPS
		imageKubeConfig.Burst = common.ImageConfigBurst
	}

	return &Gatherer{
		gatherProtoKubeConfig:       gatherProtoKubeConfig,
		metricsGatherKubeConfig:     metricsGatherKubeConfig,
		imageKubeConfig:             imageKubeConfig,
		gatherKubeConfig:            gatherKubeConfig,
		gatheringRules:              []GatheringRule{},
		configurator:                configurator,
		gatheringRulesServiceClient: gatheringRulesServiceClient,
	}
}

// GatheringRuleMetadata stores information about gathering rules
type GatheringRuleMetadata struct {
	Rule         GatheringRule `json:"rule"`
	Errors       []string      `json:"errors"`
	WasTriggered bool          `json:"was_triggered"`
}

// GetName returns the name of the gatherer
func (g *Gatherer) GetName() string {
	return "conditional"
}

// GetGatheringFunctions returns gathering functions that should be run considering the conditions
// + the gathering function producing metadata for the conditional gatherer
func (g *Gatherer) GetGatheringFunctions(ctx context.Context) (map[string]gatherers.GatheringClosure, error) {
	newGatheringRules, err := g.fetchGatheringRulesFromServer(ctx)
	if err != nil {
		klog.Errorf("unable to fetch gathering rules from the server: %v", err)
		klog.Info("trying to use cached gathering functions")
	} else {
		g.gatheringRules = newGatheringRules
	}

	errs := validateGatheringRules(g.gatheringRules)
	if len(errs) > 0 {
		return nil, fmt.Errorf("got invalid config for conditional gatherer: %v", utils.SumErrors(errs))
	}

	g.updateCache(ctx)

	gatheringFunctions := make(map[string]gatherers.GatheringClosure)

	var metadata []GatheringRuleMetadata

	for _, conditionalGathering := range g.gatheringRules {
		ruleMetadata := GatheringRuleMetadata{
			Rule: conditionalGathering,
		}

		allConditionsAreSatisfied, err := g.areAllConditionsSatisfied(conditionalGathering.Conditions)
		if err != nil {
			klog.Errorf("error checking conditions for a gathering rule: %v", err)
			ruleMetadata.Errors = append(ruleMetadata.Errors, err.Error())
		}

		ruleMetadata.WasTriggered = allConditionsAreSatisfied

		if allConditionsAreSatisfied {
			functions, errs := g.createGatheringClosures(conditionalGathering.GatheringFunctions)
			if len(errs) > 0 {
				klog.Errorf("error(s) creating a closure for a gathering rule: %v", errs)
				for _, err := range errs {
					ruleMetadata.Errors = append(ruleMetadata.Errors, err.Error())
				}
			}

			for funcName, function := range functions {
				gatheringFunctions[funcName] = function
			}
		}

		metadata = append(metadata, ruleMetadata)
	}

	gatheringFunctions["conditional_gatherer_rules"] = gatherers.GatheringClosure{
		Run: func(context.Context) ([]record.Record, []error) {
			return []record.Record{
				{
					Name: "insights-operator/conditional-gatherer-rules",
					Item: record.JSONMarshaller{Object: metadata},
				},
			}, nil
		},
	}

	return gatheringFunctions, nil
}

// fetchGatheringRulesFromServer returns the latest version of the rules from the server
func (g *Gatherer) fetchGatheringRulesFromServer(ctx context.Context) ([]GatheringRule, error) {
	gatheringRulesJSON, err := g.getGatheringRulesJSON(ctx)
	if err != nil {
		return nil, err
	}

	return parseGatheringRules(gatheringRulesJSON)
}

// getGatheringRulesJSON returns json version of the rules from the server
func (g *Gatherer) getGatheringRulesJSON(ctx context.Context) (string, error) {
	if g.configurator == nil {
		return "", fmt.Errorf("no configurator was provided")
	}

	config := g.configurator.Config()
	if config == nil {
		return "", fmt.Errorf("config is nil")
	}

	if g.gatheringRulesServiceClient == nil {
		return "", fmt.Errorf("gathering rules service client is nil")
	}

	rulesBytes, err := g.gatheringRulesServiceClient.RecvGatheringRules(ctx, config.ConditionalGathererEndpoint)
	if err != nil {
		return "", err
	}

	return string(rulesBytes), err
}

// areAllConditionsSatisfied returns true if all the conditions are satisfied, for example if the condition is
// to check if a metric is firing, it will look at that metric and return the result according to that
func (g *Gatherer) areAllConditionsSatisfied(conditions []ConditionWithParams) (bool, error) {
	for _, condition := range conditions {
		switch condition.Type {
		case AlertIsFiring:
			if condition.Alert == nil {
				return false, fmt.Errorf("alert field should not be nil")
			}

			if firing, err := g.isAlertFiring(condition.Alert.Name); !firing || err != nil {
				return false, err
			}
		case ClusterVersionMatches:
			if condition.ClusterVersionMatches == nil {
				return false, fmt.Errorf("cluster_version_matches field should not be nil")
			}

			if doesMatch, err := g.doesClusterVersionMatch(condition.ClusterVersionMatches.Version); !doesMatch || err != nil {
				return false, err
			}
		default:
			return false, fmt.Errorf("unknown condition type: %v", condition.Type)
		}
	}

	return true, nil
}

// updateCache updates alerts and version caches
func (g *Gatherer) updateCache(ctx context.Context) {
	if g.metricsGatherKubeConfig == nil {
		return
	}

	metricsClient, err := rest.RESTClientFor(g.metricsGatherKubeConfig)
	if err != nil {
		klog.Errorf("unable to update alerts cache: %v", err)
	} else if err := g.updateAlertsCache(ctx, metricsClient); err != nil { //nolint:govet
		klog.Errorf("unable to update alerts cache: %v", err)
		g.firingAlerts = nil
	}

	configClient, err := configv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		klog.Errorf("unable to update version cache: %v", err)
	} else if err := g.updateVersionCache(ctx, configClient); err != nil {
		klog.Errorf("unable to update version cache: %v", err)
		g.clusterVersion = ""
	}
}

func (g *Gatherer) updateAlertsCache(ctx context.Context, metricsClient rest.Interface) error {
	const logPrefix = "conditional gatherer: "

	g.firingAlerts = make(map[string][]AlertLabels)

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
		alertLabels := make(map[string]string)
		for _, label := range metric.GetLabel() {
			if label == nil {
				klog.Info(logPrefix + "label is nil")
				continue
			}
			alertLabels[label.GetName()] = label.GetValue()
		}
		alertName, ok := alertLabels["alertname"]
		if !ok {
			klog.Warningf("%s can't find \"alertname\" label in the metric: %v", logPrefix, metric)
			continue
		}
		g.firingAlerts[alertName] = append(g.firingAlerts[alertName], alertLabels)
	}

	return nil
}

func (g *Gatherer) updateVersionCache(ctx context.Context, configClient configv1client.ConfigV1Interface) error {
	clusterVersion, err := configClient.ClusterVersions().Get(ctx, "version", metav1.GetOptions{})
	if err != nil {
		return err
	}

	g.clusterVersion = clusterVersion.Status.Desired.Version

	return nil
}

// isAlertFiring using the cache it returns true if the alert is firing
func (g *Gatherer) isAlertFiring(alertName string) (bool, error) {
	if g.firingAlerts == nil {
		return false, fmt.Errorf("alerts cache is missing")
	}

	_, alertIsFiring := g.firingAlerts[alertName]
	return alertIsFiring, nil
}

func (g *Gatherer) doesClusterVersionMatch(expectedVersionExpression string) (bool, error) {
	if len(g.clusterVersion) == 0 {
		return false, fmt.Errorf("cluster version is missing")
	}

	clusterVersion, err := semver.Parse(g.clusterVersion)
	if err != nil {
		return false, err
	}

	expectedRange, err := semver.ParseRange(expectedVersionExpression)
	if err != nil {
		return false, err
	}

	// ignore everything after the first three numbers
	clusterVersion.Pre = nil
	clusterVersion.Build = nil

	return expectedRange(clusterVersion), nil
}

// createGatheringClosures produces gathering closures from the rules
func (g *Gatherer) createGatheringClosures(
	gatheringFunctions map[GatheringFunctionName]interface{},
) (map[string]gatherers.GatheringClosure, []error) {
	resultingClosures := make(map[string]gatherers.GatheringClosure)
	var errs []error

	for function, functionParams := range gatheringFunctions {
		builderFunc, found := gatheringFunctionBuilders[function]
		if !found {
			errs = append(errs, fmt.Errorf("unknown action type: %v", function))
			continue
		}

		closure, err := builderFunc(g, functionParams)
		if err != nil {
			errs = append(errs, err)
		} else {
			name, err := getConditionalGatheringFunctionName(string(function), functionParams)
			if err != nil {
				errs = append(errs, fmt.Errorf(
					"unable to get name for the function %v with params %v: %v",
					function, functionParams, err,
				))
				continue
			}
			resultingClosures[name] = closure
		}
	}

	return resultingClosures, errs
}

// getConditionalGatheringFunctionName creates a name of the conditional gathering function adding the parameters
// after the name. For example:
//   "conditional/logs_of_namespace/namespace=openshift-cluster-samples-operator,tail_lines=100"
func getConditionalGatheringFunctionName(funcName string, gatherParamsInterface interface{}) (string, error) {
	gatherParams, err := utils.StructToMap(gatherParamsInterface)
	if err != nil {
		return "", err
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

	return funcName, nil
}
