package configobserver

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/config"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringcli "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
)

type ConfigReporter interface {
	SetConfig(*config.Controller)
}

type Configurator interface {
	Config() *config.Controller
	ConfigChanged() (<-chan struct{}, func())
}

// Controller is responsible for periodically checking and (if necessary) updating the local configs/tokens
// according to the configs/tokens present on the cluster.
type Controller struct {
	kubeClient kubernetes.Interface

	lock            sync.Mutex
	defaultConfig   config.Controller
	tokenConfig     *config.Controller
	secretConfig    *config.Controller
	supportSecret   *v1.Secret
	config          *config.Controller
	checkPeriod     time.Duration
	listeners       []chan struct{}
	monitoringCli   monitoringcli.Clientset
	promRulesExists bool
}

// New creates a new configobsever, the configs/tokens are updated from the configs/tokens present in the cluster if possible.
func New(defaultConfig config.Controller, kubeClient kubernetes.Interface, kubeConfig *rest.Config) *Controller { //nolint: gocritic
	monitoringCS, err := monitoringcli.NewForConfig(kubeConfig)
	if err != nil {
		klog.Warningf("Unable create monitoring client: %v", err)
	}
	c := &Controller{
		kubeClient:    kubeClient,
		defaultConfig: defaultConfig,
		checkPeriod:   5 * time.Minute,
		monitoringCli: *monitoringCS,
	}
	c.mergeConfig()
	if err := c.updateToken(context.TODO()); err != nil {
		klog.Warningf("Unable to retrieve initial token config: %v", err)
	}
	if err := c.updateConfig(context.TODO()); err != nil {
		klog.Warningf("Unable to retrieve initial config: %v", err)
	}
	return c
}

// Start is periodically invoking check and set of config and token
func (c *Controller) Start(ctx context.Context) {
	configCh, cancelFn := c.ConfigChanged()
	defer cancelFn()

	c.checkAlertsDisabled(ctx)

	for {
		select {
		case <-time.After(c.checkPeriod):
			if err := c.updateToken(ctx); err != nil {
				klog.Warningf("Unable to retrieve token config: %v", err)
			}
			if err := c.updateConfig(ctx); err != nil {
				klog.Warningf("Unable to retrieve config: %v", err)
			}
		case <-configCh:
			c.checkAlertsDisabled(ctx)
		case <-ctx.Done():
			return
		}
		if err := c.updateConfig(ctx); err != nil {
			klog.Warningf("Unable to retrieve config: %v", err)
		}
	}
}

// Config provides the config in a thread-safe way.
func (c *Controller) Config() *config.Controller {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.config
}

func (c *Controller) SupportSecret() *v1.Secret {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.supportSecret
}

// ConfigChanged subscribe for config changes
// 1.Param: A channel where the listener is notified that the config has changed.
// 2.Param: A func which can be used to unsubscribe from the config changes.
func (c *Controller) ConfigChanged() (configCh <-chan struct{}, closeFn func()) {
	c.lock.Lock()
	defer c.lock.Unlock()
	position := -1
	for i := range c.listeners {
		if c.listeners == nil {
			position = i
			break
		}
	}
	if position == -1 {
		c.listeners = append(c.listeners, nil)
		position = len(c.listeners) - 1
	}
	ch := make(chan struct{}, 1)
	c.listeners[position] = ch
	return ch, func() {
		c.lock.Lock()
		defer c.lock.Unlock()
		c.listeners[position] = nil
	}
}

// Fetches the token from cluster secret key
func (c *Controller) fetchSecret(ctx context.Context, name string) (*v1.Secret, error) {
	secret, err := c.kubeClient.CoreV1().Secrets("openshift-config").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(4).Infof("%s secret does not exist", name)
			err = nil
			secret = nil
		} else if errors.IsForbidden(err) {
			klog.V(2).Infof("Operator does not have permission to check %s: %v", name, err)
			err = nil
			secret = nil
		} else {
			err = fmt.Errorf("could not check %s: %v", name, err)
		}
	}

	return secret, err
}

// Updates the stored tokens from the secrets in the cluster. (if present)
func (c *Controller) updateToken(ctx context.Context) error {
	klog.V(2).Infof("Refreshing configuration from cluster pull secret")
	secret, err := c.fetchSecret(ctx, "pull-secret")
	if err != nil {
		return err
	}

	var nextConfig config.Controller
	if secret != nil {
		var token string
		token, err = tokenFromSecret(secret)
		if err != nil {
			return err
		}
		if len(token) > 0 {
			nextConfig.Token = token
			nextConfig.Report = true
		}
	}

	c.setTokenConfig(&nextConfig)

	return nil
}

// Updates the stored configs from the secrets in the cluster. (if present)
func (c *Controller) updateConfig(ctx context.Context) error {
	klog.V(2).Infof("Refreshing configuration from cluster secret")
	secret, err := c.fetchSecret(ctx, "support")
	if err != nil {
		return err
	}

	c.supportSecret = secret
	if secret == nil {
		c.setSecretConfig(nil)
	} else {
		nextConfig, err := LoadConfigFromSecret(secret)
		if err != nil {
			return err
		}

		c.setSecretConfig(&nextConfig)
	}

	return nil
}

// Sets the token configuration to the observer
func (c *Controller) setTokenConfig(operatorConfig *config.Controller) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.tokenConfig = operatorConfig
	c.mergeConfig()
}

// Sets the secret configuration to the observer
func (c *Controller) setSecretConfig(operatorConfig *config.Controller) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.secretConfig = operatorConfig
	c.mergeConfig()
}

// Sets the operator configuration to the observer
func (c *Controller) setConfig(operatorConfig *config.Controller) {
	if c.config != nil {
		if !reflect.DeepEqual(c.config, operatorConfig) {
			klog.V(2).Infof("Configuration updated: %s", operatorConfig.ToString())
			for _, ch := range c.listeners {
				if ch == nil {
					continue
				}
				select {
				case ch <- struct{}{}:
				default:
				}
			}
		}
	} else {
		klog.V(2).Infof("Configuration set: %s", operatorConfig.ToString())
	}
	c.config = operatorConfig
}

// Merges operator configuration to the observer
func (c *Controller) mergeConfig() {
	cfg := c.defaultConfig

	if c.secretConfig != nil {
		cfg.MergeWith(c.secretConfig)
	}
	if c.tokenConfig != nil {
		cfg.Token = c.tokenConfig.Token
	}

	cfg.Report = len(cfg.Endpoint) > 0 && (len(cfg.Token) > 0 || len(cfg.Username) > 0)
	c.setConfig(&cfg)
}

// Parses the given secret to retrieve the token
func tokenFromSecret(secret *v1.Secret) (string, error) {
	if data := secret.Data[".dockerconfigjson"]; len(data) > 0 {
		var pullSecret serializedAuthMap
		if err := json.Unmarshal(data, &pullSecret); err != nil {
			klog.Errorf("Unable to unmarshal cluster pull-secret: %v", err)
		}
		if auth, ok := pullSecret.Auths["cloud.openshift.com"]; ok {
			token := strings.TrimSpace(auth.Auth)
			if strings.Contains(token, "\n") || strings.Contains(token, "\r") {
				return "", fmt.Errorf("cluster authorization token is not valid: contains newlines")
			}
			if len(token) > 0 {
				klog.V(4).Info("Found cloud.openshift.com token")
				return token, nil
			}
		}
	}
	return "", nil
}

type serializedAuthMap struct {
	Auths map[string]serializedAuth `json:"auths"`
}
type serializedAuth struct {
	Auth string `json:"auth"`
}

func (c *Controller) createInsightsAlerts(ctx context.Context) error {
	pr := &monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "insights-prometheus-rules",
			Namespace: "openshift-insights",
		},
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: []monitoringv1.RuleGroup{
				{
					Name: "insights",
					Rules: []monitoringv1.Rule{
						{
							Alert: "InsightsDisabled",
							Expr:  intstr.FromString("max without (job, pod, service, instance) (cluster_operator_conditions{name=\"insights\", condition=\"Disabled\"} == 1)"),
							For:   monitoringv1.Duration("5m"),
							Labels: map[string]string{
								"severity":  "info",
								"namespace": "openshift-insights",
							},
							Annotations: map[string]string{
								"description": "Insights operator is disabled. In order to enable Insights and benefit from recommendations specific to your cluster, please follow steps listed in the documentation: https://docs.openshift.com/container-platform/latest/support/remote_health_monitoring/enabling-remote-health-reporting.html",
								"summary":     "Insights operator is disabled.",
							},
						},
						{
							Alert: "SimpleContentAccessNotAvailable",
							Expr:  intstr.FromString(" max without (job, pod, service, instance) (max_over_time(cluster_operator_conditions{name=\"insights\", condition=\"SCAAvailable\", reason=\"NotFound\"}[5m]) == 0)"),
							For:   monitoringv1.Duration("5m"),
							Labels: map[string]string{
								"severity":  "info",
								"namespace": "openshift-insights",
							},
							Annotations: map[string]string{
								"description": "Simple content access (SCA) is not enabled. Once enabled, Insights Operator can automatically import the SCA certificates from Red Hat OpenShift Cluster Manager making it easier to use the content provided by your Red Hat subscriptions when creating container images. See https://docs.openshift.com/container-platform/latest/cicd/builds/running-entitled-builds.html for more information.",
								"summary":     "Simple content access certificates are not available.",
							},
						},
						{
							Alert: "InsightsRecommendationActive",
							Expr:  intstr.FromString("insights_recommendation_active == 1"),
							For:   monitoringv1.Duration("5m"),
							Labels: map[string]string{
								"severity": "info",
							},
							Annotations: map[string]string{
								"description": "Insights recommendation \"{{ $labels.description }}\" with total risk \"{{ $labels.total_risk }}\" was detected on the cluster. More information is available at {{ $labels.info_link }}.",
								"summary":     "An Insights recommendation is active for this cluster.",
							},
						},
					},
				},
			},
		},
	}

	_, err := c.monitoringCli.MonitoringV1().PrometheusRules("openshift-insights").Create(ctx, pr, metav1.CreateOptions{})
	return err
}

func (c *Controller) removeInsightsAlerts(ctx context.Context) error {
	return c.monitoringCli.MonitoringV1().
		PrometheusRules("openshift-insights").
		Delete(ctx, "insights-prometheus-rules", metav1.DeleteOptions{})
}

func (c *Controller) checkAlertsDisabled(ctx context.Context) {
	if c.Config().DisableInsightsAlerts {
		if c.promRulesExists {
			err := c.removeInsightsAlerts(ctx)
			if err != nil {
				klog.Errorf("Failed to remove Insights Prometheus rules definition: %v ", err)
			} else {
				klog.Info("Prometheus rules successfully removed")
				c.promRulesExists = false
			}
		}
	} else {
		if !c.promRulesExists {
			err := c.createInsightsAlerts(ctx)
			if err != nil {
				klog.Errorf("Failed to create Insights Prometheus rules definition: %v ", err)
			} else {
				klog.Info("Prometheus rules successfully created")
				c.promRulesExists = true
			}
		}
	}
}
