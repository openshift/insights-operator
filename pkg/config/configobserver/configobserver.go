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

// Controller is responsible for periodically checking and (if necessary) updating the local configs/tokens
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

// New creates a new configobsever, the configs/tokens are updated from the configs/tokens present in the cluster if possible.
func New(defaultConfig config.Controller, kubeClient kubernetes.Interface) *Controller { //nolint: gocritic
	c := &Controller{
		kubeClient:    kubeClient,
		defaultConfig: defaultConfig,
		checkPeriod:   5 * time.Minute,
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
	wait.Until(func() {
		if err := c.updateToken(ctx); err != nil {
			klog.Warningf("Unable to retrieve token config: %v", err)
		}
		if err := c.updateConfig(ctx); err != nil {
			klog.Warningf("Unable to retrieve config: %v", err)
		}
	}, c.checkPeriod, ctx.Done())
}

// Config provides the config in a thread-safe way.
func (c *Controller) Config() *config.Controller {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.config
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
func (c *Controller) fetchSecret(ctx context.Context, key string) (*v1.Secret, error) {
	secret, err := c.kubeClient.CoreV1().Secrets("openshift-config").Get(ctx, key, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(4).Infof("%s secret does not exist", key)
			err = nil
		} else if errors.IsForbidden(err) {
			klog.V(2).Infof("Operator does not have permission to check %s: %v", key, err)
			err = nil
		} else {
			err = fmt.Errorf("could not check %s: %v", key, err)
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
	var nextConfig config.Controller
	klog.V(2).Infof("Refreshing configuration from cluster secret")
	secret, err := c.fetchSecret(ctx, "support")
	if err != nil {
		return err
	}

	if secret != nil {
		nextConfig, err = LoadConfigFromSecret(secret)
		if err != nil {
			return err
		}
	}

	c.setSecretConfig(&nextConfig)

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
