// Package conditional provides conditional gatherer which runs gatherings based on the rules and only if the provided
// conditions are satisfied. The rules are fetched from Insights Operator Gathering Conditions Service
// https://github.com/RedHatInsights/insights-operator-gathering-conditions-service . The rules are validated to
// check that they make sense (for example we don't allow collecting logs from non openshift namespaces).
//
// To add a new condition, follow the steps described in conditions.go file.
// To add a new gathering function, follow the steps described in gathering_functions.go file.
package conditional

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
)

// Gatherer implements the conditional gatherer
type Gatherer struct {
	gatherProtoKubeConfig   *rest.Config
	metricsGatherKubeConfig *rest.Config
	imageKubeConfig         *rest.Config
	gatherKubeConfig        *rest.Config
	// there can be multiple instances of the same alert
	firingAlerts                map[string][]AlertLabels
	gatheringRules              GatheringRules
	clusterVersion              string
	configurator                configobserver.Interface
	gatheringRulesServiceClient GatheringRulesServiceClient
}

type GatheringRulesServiceClient interface {
	RecvGatheringRules(ctx context.Context, endpoint string) ([]byte, error)
}

// New creates a new instance of conditional gatherer with the appropriate configs
func New(
	gatherProtoKubeConfig, metricsGatherKubeConfig, gatherKubeConfig *rest.Config,
	configurator configobserver.Interface, gatheringRulesServiceClient GatheringRulesServiceClient,
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
		gatheringRules:              GatheringRules{},
		configurator:                configurator,
		gatheringRulesServiceClient: gatheringRulesServiceClient,
	}
}

// GatheringRuleMetadata stores metadata about a gathering rule
type GatheringRuleMetadata struct {
	Rule         GatheringRule `json:"rule"`
	Errors       []string      `json:"errors"`
	WasTriggered bool          `json:"was_triggered"`
}

// GatheringRulesMetadata stores metadata about gathering rules
type GatheringRulesMetadata struct {
	Version  string                  `json:"version"`
	Rules    []GatheringRuleMetadata `json:"rules"`
	Endpoint string                  `json:"endpoint"`
}

// GetName returns the name of the gatherer
func (g *Gatherer) GetName() string {
	return "conditional"
}

// GetGatheringFunctions returns gathering functions that should be run considering the conditions
// + the gathering function producing metadata for the conditional gatherer
func (g *Gatherer) GetGatheringFunctions(ctx context.Context) (map[string]gatherers.GatheringClosure, error) {
	newGatheringRules, err := g.fetchGatheringRulesFromServer(ctx)
	klog.Infof(
		"got %v gathering rules for conditional gatherer with version %v",
		len(newGatheringRules.Rules), newGatheringRules.Version,
	)
	if err != nil {
		klog.Errorf("unable to fetch gathering rules from the server: %v", err)
		klog.Infof(
			"trying to use cached gathering config containing %v gathering rules and version %v",
			len(g.gatheringRules.Rules), g.gatheringRules.Version,
		)

		return g.createGatheringFunctions(ctx)
	}

	g.gatheringRules = newGatheringRules

	return g.createGatheringFunctions(ctx)
}

func (g *Gatherer) createGatheringFunctions(ctx context.Context) (map[string]gatherers.GatheringClosure, error) {
	errs := validateGatheringRules(g.gatheringRules.Rules)
	if len(errs) > 0 {
		return nil, fmt.Errorf("got invalid config for conditional gatherer: %v", utils.SumErrors(errs))
	}

	g.updateCache(ctx)

	gatheringFunctions := make(map[string]gatherers.GatheringClosure)

	endpoint, err := g.getRulesEndpoint()
	if err != nil {
		klog.Error(err)
	}

	metadata := GatheringRulesMetadata{
		Version:  g.gatheringRules.Version,
		Endpoint: endpoint,
	}

	for _, conditionalGathering := range g.gatheringRules.Rules {
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

		metadata.Rules = append(metadata.Rules, ruleMetadata)
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
func (g *Gatherer) fetchGatheringRulesFromServer(ctx context.Context) (GatheringRules, error) {
	gatheringRulesJSON, err := g.getGatheringRulesJSON(ctx)
	if err != nil {
		return GatheringRules{}, err
	}
	// if gatheringRulesJson has invalid json format and cannot be unmarshalled no rules will be returned
	return parseGatheringRules(gatheringRulesJSON)
}

// getGatheringRulesJSON returns json version of the rules from the server
func (g *Gatherer) getGatheringRulesJSON(ctx context.Context) (string, error) {
	if g.configurator == nil {
		return "", fmt.Errorf("no configurator was provided")
	}

	if g.gatheringRulesServiceClient == nil {
		return "", fmt.Errorf("gathering rules service client is nil")
	}

	endpoint, err := g.getRulesEndpoint()
	if err != nil {
		return "", err
	}

	rulesBytes, err := g.gatheringRulesServiceClient.RecvGatheringRules(ctx, endpoint)
	if err != nil {
		return "", err
	}

	return string(rulesBytes), err
}
func (g *Gatherer) getRulesEndpoint() (string, error) {
	config := g.configurator.Config()
	if config == nil {
		return "", fmt.Errorf("config is nil")
	}

	return config.DataReporting.ConditionalGathererEndpoint, nil
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
	klog.Info("updating alerts cache for conditional gatherer")

	g.firingAlerts = make(map[string][]AlertLabels)

	data, err := metricsClient.Get().
		AbsPath("api/v1/query").
		Param("query", "ALERTS").
		Param("match[]", `ALERTS{alertstate="firing"}`).
		DoRaw(ctx)
	if err != nil {
		return err
	}

	var response struct {
		Data struct {
			Results []struct {
				Labels map[string]string `json:"metric"`
			} `json:"result"`
		} `json:"data"`
	}
	err = json.Unmarshal(data, &response)
	if err != nil {
		return err
	}

	for _, result := range response.Data.Results {
		alertName, found := result.Labels["alertname"]
		if !found {
			klog.Errorf(`label "alertname" was not found in the result: %v`, result)
			continue
		}
		alertState, found := result.Labels["alertstate"]
		if !found {
			klog.Errorf(`label "alertstate" was not found in the result: %v`, result)
			continue
		}
		klog.Infof(`alert "%v" has state "%v"`, alertName, alertState)
		if alertState == "firing" {
			g.firingAlerts[alertName] = append(g.firingAlerts[alertName], result.Labels)
		}
	}

	return nil
}

func (g *Gatherer) updateVersionCache(ctx context.Context, configClient configv1client.ConfigV1Interface) error {
	klog.Info("updating version cache for conditional gatherer")

	clusterVersion, err := configClient.ClusterVersions().Get(ctx, "version", metav1.GetOptions{})
	if err != nil {
		return err
	}

	g.clusterVersion = clusterVersion.Status.Desired.Version
	klog.Infof("cluster version is '%v'", g.clusterVersion)
	return nil
}

// createGatheringClosures produces gathering closures
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
//
//	"conditional/logs_of_namespace/namespace=openshift-cluster-samples-operator,tail_lines=100"
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
