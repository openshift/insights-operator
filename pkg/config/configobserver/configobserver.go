package configobserver

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/config"
)

type ConfigReporter interface {
	SetConfig(*config.Controller)
}

type Configurator interface {
	Config() *config.Controller
	ConfigChanged() (<-chan struct{}, func())
}

// Responsible for periodically checking and (if necessary) updating the local configs/tokens
// according to the configs/tokens present on the cluster.
type Controller struct {
	kubeClient kubernetes.Interface

	lock          sync.Mutex
	defaultConfig config.Controller
	tokenConfig   *config.Controller
	secretConfig  *config.Controller
	config        *config.Controller
	checkPeriod   time.Duration
	listeners     []chan struct{}
}

// Creates a new configobsever, the configs/tokens are updated from the configs/tokens present in the cluster if possible.
func New(defaultConfig config.Controller, kubeClient kubernetes.Interface) *Controller { //nolint: gocritic
	c := &Controller{
		kubeClient:    kubeClient,
		defaultConfig: defaultConfig,
		checkPeriod:   5 * time.Minute,
	}
	c.mergeConfigLocked()
	if err := c.retrieveToken(context.TODO()); err != nil {
		klog.Warningf("Unable to retrieve initial token config: %v", err)
	}
	if err := c.retrieveConfig(context.TODO()); err != nil {
		klog.Warningf("Unable to retrieve initial config: %v", err)
	}
	return c
}

// Start is periodically invoking check and set of config and token
func (c *Controller) Start(ctx context.Context) {
	wait.Until(func() {
		if err := c.retrieveToken(ctx); err != nil {
			klog.Warningf("Unable to retrieve token config: %v", err)
		}
		if err := c.retrieveConfig(ctx); err != nil {
			klog.Warningf("Unable to retrieve config: %v", err)
		}
	}, c.checkPeriod, ctx.Done())
}

// Updates the stored tokens from the secrets in the cluster. (if present)
func (c *Controller) retrieveToken(ctx context.Context) error {
	var nextConfig config.Controller

	klog.V(2).Infof("Refreshing configuration from cluster pull secret")
	secret, err := c.kubeClient.CoreV1().Secrets("openshift-config").Get(ctx, "pull-secret", metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(4).Infof("pull-secret does not exist")
			err = nil
		} else if errors.IsForbidden(err) {
			klog.V(2).Infof("Operator does not have permission to check pull-secret: %v", err)
			err = nil
		} else {
			err = fmt.Errorf("could not check pull-secret: %v", err)
		}
	}
	if secret != nil {
		if data := secret.Data[".dockerconfigjson"]; len(data) > 0 {
			var pullSecret serializedAuthMap
			if err := json.Unmarshal(data, &pullSecret); err != nil { //nolint: govet
				klog.Errorf("Unable to unmarshal cluster pull-secret: %v", err)
			}
			if auth, ok := pullSecret.Auths["cloud.openshift.com"]; ok {
				token := strings.TrimSpace(auth.Auth)
				if strings.Contains(token, "\n") || strings.Contains(token, "\r") {
					return fmt.Errorf("cluster authorization token is not valid: contains newlines")
				}
				if len(token) > 0 {
					klog.V(4).Info("Found cloud.openshift.com token")
					nextConfig.Token = token
				}
			}
		}
		nextConfig.Report = true
	}
	if err != nil {
		return err
	}
	c.setTokenConfig(&nextConfig)
	return nil
}

// Updates the stored configs from the secrets in the cluster. (if present)
func (c *Controller) retrieveConfig(ctx context.Context) error { //nolint: gocyclo
	var nextConfig config.Controller

	klog.V(2).Infof("Refreshing configuration from cluster secret")
	secret, err := c.kubeClient.CoreV1().Secrets("openshift-config").Get(ctx, "support", metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(4).Infof("Support secret does not exist")
			err = nil
		} else if errors.IsForbidden(err) {
			klog.V(2).Infof("Operator does not have permission to check support secret: %v", err)
			err = nil
		} else {
			err = fmt.Errorf("could not check support secret: %v", err)
		}
	}
	if secret != nil {
		if username, ok := secret.Data["username"]; ok {
			nextConfig.Username = string(username)
		}
		if password, ok := secret.Data["password"]; ok {
			nextConfig.Password = string(password)
		}
		if endpoint, ok := secret.Data["endpoint"]; ok {
			nextConfig.Endpoint = string(endpoint)
		}
		if httpproxy, ok := secret.Data["httpProxy"]; ok {
			nextConfig.HTTPConfig.HTTPProxy = string(httpproxy)
		}
		if httpsproxy, ok := secret.Data["httpsProxy"]; ok {
			nextConfig.HTTPConfig.HTTPSProxy = string(httpsproxy)
		}
		if noproxy, ok := secret.Data["noProxy"]; ok {
			nextConfig.HTTPConfig.NoProxy = string(noproxy)
		}
		if reportEndpoint, ok := secret.Data["reportEndpoint"]; ok {
			nextConfig.ReportEndpoint = string(reportEndpoint)
		}
		if enableGlobalObfuscation, ok := secret.Data["enableGlobalObfuscation"]; ok {
			nextConfig.EnableGlobalObfuscation = strings.EqualFold(string(enableGlobalObfuscation), "true")
		}
		if reportPullingDelay, ok := secret.Data["reportPullingDelay"]; ok {
			if v, err := time.ParseDuration(string(reportPullingDelay)); err == nil { //nolint: govet
				nextConfig.ReportPullingDelay = v
			} else {
				klog.Warningf(
					"reportPullingDelay secret contains an invalid value (%s). Using previous value",
					reportPullingDelay,
				)
			}
		} else {
			nextConfig.ReportPullingDelay = time.Duration(-1)
		}
		if reportPullingTimeout, ok := secret.Data["reportPullingTimeout"]; ok {
			if v, err := time.ParseDuration(string(reportPullingTimeout)); err == nil { //nolint: govet
				nextConfig.ReportPullingTimeout = v
			} else {
				klog.Warningf(
					"reportPullingTimeout secret contains an invalid value (%s). Using previous value",
					reportPullingTimeout,
				)
			}
		}
		if reportMinRetryTime, ok := secret.Data["reportMinRetryTime"]; ok {
			if v, err := time.ParseDuration(string(reportMinRetryTime)); err == nil { //nolint: govet
				nextConfig.ReportMinRetryTime = v
			} else {
				klog.Warningf(
					"reportMinRetryTime secret contains an invalid value (%s). Using previous value",
					reportMinRetryTime,
				)
			}
		}
		nextConfig.Report = len(nextConfig.Endpoint) > 0

		if intervalString, ok := secret.Data["interval"]; ok {
			var duration time.Duration
			duration, err = time.ParseDuration(string(intervalString))
			if err == nil && duration < 10*time.Second {
				err = fmt.Errorf("too short")
			}
			if err == nil {
				nextConfig.Interval = duration
			} else {
				err = fmt.Errorf("insights secret interval must be a duration (1h, 10m) greater than or equal to ten seconds: %v", err)
				nextConfig.Report = false
			}
		}
	}
	if err != nil {
		return err
	}
	c.setSecretConfig(&nextConfig)
	return nil
}

// Provides the config in a thread-safe way.
func (c *Controller) Config() *config.Controller {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.config
}

// Subscribe for config changes
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

func (c *Controller) setTokenConfig(operatorConfig *config.Controller) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.tokenConfig = operatorConfig
	c.mergeConfigLocked()
}

func (c *Controller) setSecretConfig(operatorConfig *config.Controller) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.secretConfig = operatorConfig
	c.mergeConfigLocked()
}

func (c *Controller) mergeConfigLocked() {
	cfg := c.defaultConfig
	if c.secretConfig != nil {
		cfg.Username = c.secretConfig.Username
		cfg.Password = c.secretConfig.Password
		if c.secretConfig.Interval > 0 {
			cfg.Interval = c.secretConfig.Interval
		}
		if len(c.secretConfig.Endpoint) > 0 {
			cfg.Endpoint = c.secretConfig.Endpoint
		}
		if len(c.secretConfig.ReportEndpoint) > 0 {
			cfg.ReportEndpoint = c.secretConfig.ReportEndpoint
		}
		if c.secretConfig.ReportPullingDelay >= 0 {
			cfg.ReportPullingDelay = c.secretConfig.ReportPullingDelay
		}
		if c.secretConfig.ReportPullingTimeout > 0 {
			cfg.ReportPullingTimeout = c.secretConfig.ReportPullingTimeout
		}
		if c.secretConfig.ReportMinRetryTime > 0 {
			cfg.ReportMinRetryTime = c.secretConfig.ReportMinRetryTime
		}
		cfg.EnableGlobalObfuscation = cfg.EnableGlobalObfuscation || c.secretConfig.EnableGlobalObfuscation
		cfg.HTTPConfig = c.secretConfig.HTTPConfig
	}
	if c.tokenConfig != nil {
		cfg.Token = c.tokenConfig.Token
	}
	cfg.Report = len(cfg.Endpoint) > 0 && (len(cfg.Token) > 0 || len(cfg.Username) > 0)
	c.setConfigLocked(&cfg)
}

func (c *Controller) setConfigLocked(operatorConfig *config.Controller) {
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

type serializedAuthMap struct {
	Auths map[string]serializedAuth `json:"auths"`
}
type serializedAuth struct {
	Auth string `json:"auth"`
}
