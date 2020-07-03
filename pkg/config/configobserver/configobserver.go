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
	"k8s.io/klog"

	"github.com/openshift/insights-operator/pkg/config"
)

type ConfigReporter interface {
	SetConfig(*config.Controller)
}

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

func New(defaultConfig config.Controller, kubeClient kubernetes.Interface) *Controller {
	c := &Controller{
		kubeClient:    kubeClient,
		defaultConfig: defaultConfig,
		checkPeriod:   5 * time.Minute,
	}
	c.mergeConfigLocked()
	if err := c.retrieveToken(); err != nil {
		klog.Warningf("Unable to retrieve initial token config: %v", err)
	}
	if err := c.retrieveConfig(); err != nil {
		klog.Warningf("Unable to retrieve initial config: %v", err)
	}
	return c
}

// Start is periodically invoking check and set of config and token
func (c *Controller) Start(ctx context.Context) {
	wait.Until(func() {
		if err := c.retrieveToken(); err != nil {
			klog.Warningf("Unable to retrieve token config: %v", err)
		}
		if err := c.retrieveConfig(); err != nil {
			klog.Warningf("Unable to retrieve config: %v", err)
		}
	}, c.checkPeriod, ctx.Done())
}

func (c *Controller) retrieveToken() error {
	var nextConfig config.Controller

	klog.V(2).Infof("Refreshing configuration from cluster pull secret")
	secret, err := c.kubeClient.CoreV1().Secrets("openshift-config").Get("pull-secret", metav1.GetOptions{})
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
			if err := json.Unmarshal(data, &pullSecret); err != nil {
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

func (c *Controller) retrieveConfig() error {
	var nextConfig config.Controller

	klog.V(2).Infof("Refreshing configuration from cluster secret")
	secret, err := c.kubeClient.CoreV1().Secrets("openshift-config").Get("support", metav1.GetOptions{})
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
		nextConfig.Report = len(nextConfig.Endpoint) > 0

		if intervalString, ok := secret.Data["interval"]; ok {
			var duration time.Duration
			duration, err = time.ParseDuration(string(intervalString))
			if err == nil && duration < time.Minute {
				err = fmt.Errorf("too short")
			}
			if err == nil {
				nextConfig.Interval = duration
			} else {
				err = fmt.Errorf("insights secret interval must be a duration (1h, 10m) greater than or equal to one minute: %v", err)
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

func (c *Controller) Config() *config.Controller {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.config
}

func (c *Controller) ConfigChanged() (<-chan struct{}, func()) {
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

func (c *Controller) setTokenConfig(config *config.Controller) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.tokenConfig = config
	c.mergeConfigLocked()
}

func (c *Controller) setSecretConfig(config *config.Controller) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.secretConfig = config
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
		cfg.HTTPConfig = c.secretConfig.HTTPConfig
	}
	if c.tokenConfig != nil {
		cfg.Token = c.tokenConfig.Token
	}
	cfg.Report = len(cfg.Endpoint) > 0 && (len(cfg.Token) > 0 || len(cfg.Username) > 0)
	c.setConfigLocked(&cfg)
}

func (c *Controller) setConfigLocked(config *config.Controller) {
	if c.config != nil {
		if !reflect.DeepEqual(c.config, config) {
			klog.V(2).Infof("Configuration updated: enabled=%t endpoint=%s interval=%s username=%t token=%t", config.Report, config.Endpoint, config.Interval, len(config.Username) > 0, len(config.Token) > 0)
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
		klog.V(2).Infof("Configuration set: enabled=%t endpoint=%s interval=%s username=%t token=%t", config.Report, config.Endpoint, config.Interval, len(config.Username) > 0, len(config.Token) > 0)
	}
	c.config = config
}

type serializedAuthMap struct {
	Auths map[string]serializedAuth `json:"auths"`
}
type serializedAuth struct {
	Auth string `json:"auth"`
}
