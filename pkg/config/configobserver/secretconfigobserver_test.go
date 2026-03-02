package configobserver

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clsetfake "k8s.io/client-go/kubernetes/fake"
	corefake "k8s.io/client-go/kubernetes/typed/core/v1/fake"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/utils"

	clienttesting "k8s.io/client-go/testing"
)

type kubeClientResponder struct {
	clsetfake.Clientset
}

const (
	pullSecretKey = "(/v1, Resource=secrets) openshift-config.pull-secret" //nolint: gosec
	supportKey    = "(/v1, Resource=secrets) openshift-config.support"
)

// nolint: lll, funlen
func Test_ConfigObserver_ChangeSupportConfig(t *testing.T) {
	cases := []struct {
		name      string
		config    map[string]*corev1.Secret
		expConfig *config.Controller
		expErr    error
	}{
		{
			name: "interval too short",
			config: map[string]*corev1.Secret{
				supportKey: {Data: map[string][]byte{
					"username": []byte("someone"),
					"password": []byte("secret"),
					"endpoint": []byte("http://po.rt"),
					"interval": []byte("1s"),
				}},
			},
			expErr: fmt.Errorf("interval value too short, minimal value is 10 minutes"),
		},
		{
			name: "interval incorrect format",
			config: map[string]*corev1.Secret{
				supportKey: {Data: map[string][]byte{
					"interval": []byte("every second"),
				}},
			},
			expErr: fmt.Errorf("insights secret interval must be a duration (1h, 10m) greater than or equal to ten minutes: time: invalid duration \"every second\""),
		},
		{
			name: "reportPullingDelay incorrect format",
			config: map[string]*corev1.Secret{
				supportKey: {Data: map[string][]byte{
					"reportPullingDelay": []byte("every second"),
				}},
			},
			expConfig: &config.Controller{}, // it only produces a warning in the log
		},
		{
			name: "reportMinRetryTime incorrect format",
			config: map[string]*corev1.Secret{
				supportKey: {Data: map[string][]byte{
					"reportMinRetryTime": []byte("every second"),
				}},
			},
			expConfig: &config.Controller{}, // it only produces a warning in the log
		},
		{
			name: "reportPullingTimeout incorrect format",
			config: map[string]*corev1.Secret{
				supportKey: {Data: map[string][]byte{
					"reportPullingTimeout": []byte("every second"),
				}},
			},
			expConfig: &config.Controller{}, // it only produces a warning in the log
		},
		{
			name: "correct interval",
			config: map[string]*corev1.Secret{
				supportKey: {Data: map[string][]byte{
					"interval": []byte("15m"),
				}},
			},
			expConfig: &config.Controller{
				Interval: 15 * time.Minute,
			},
			expErr: nil,
		},
		{
			name: "set-all-config",
			config: map[string]*corev1.Secret{
				pullSecretKey: {Data: map[string][]byte{
					".dockerconfigjson": []byte(`{"auths":{"cloud.openshift.com":{"auth":"testtoken","email":"test"}}}`),
				}},

				supportKey: {Data: map[string][]byte{
					"endpoint":                []byte("http://po.rt"),
					"httpProxy":               []byte("http://pro.xy"),
					"httpsProxy":              []byte("https://pro.xy"),
					"noProxy":                 []byte("http://no.xy"),
					"enableGlobalObfuscation": []byte("true"),
					"reportEndpoint":          []byte("http://rep.rt"),
					"reportPullingDelay":      []byte("10m"),
					"reportMinRetryTime":      []byte("10m"),
					"reportPullingTimeout":    []byte("10m"),
					"interval":                []byte("10m"),
				}},
			},
			expConfig: &config.Controller{
				Report:   true,
				Endpoint: "http://po.rt",
				Token:    "testtoken",
				HTTPConfig: config.HTTPConfig{
					HTTPProxy:  "http://pro.xy",
					HTTPSProxy: "https://pro.xy",
					NoProxy:    "http://no.xy",
				},
				EnableGlobalObfuscation: true,
				ReportEndpoint:          "http://rep.rt",
				ReportPullingDelay:      10 * time.Minute,
				ReportMinRetryTime:      10 * time.Minute,
				ReportPullingTimeout:    10 * time.Minute,
				Interval:                10 * time.Minute,
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// log klog to test
			klog.SetOutput(utils.NewTestLog(t).Writer())

			ctrl := config.Controller{}
			kube := kubeClientResponder{}
			// setup mock responses for secretes by secret name
			provideSecretMock(&kube, tt.config)

			// imitates New function
			c := &Controller{
				kubeClient:    &kube,
				defaultConfig: ctrl,
			}
			c.mergeConfig()

			err := c.updateToken(context.Background())
			if err == nil {
				err = c.updateConfig(context.Background())
			}

			if tt.expErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expErr, err)
				return
			}

			assert.Equal(t, tt.expConfig, c.config)
		})
	}
}

func Test_ConfigObserver_ConfigChanged(t *testing.T) {
	ctrl := config.Controller{}
	kube := kubeClientResponder{}
	// Imitates New function
	c := &Controller{
		kubeClient:    &kube,
		defaultConfig: ctrl,
	}
	c.mergeConfig()

	// Subscribe to config change event
	configCh, closeFn := c.ConfigChanged()
	if len(configCh) > 0 {
		t.Fatalf("Config channel is not empty on start.")
	}
	// Setup mock for 1. config update
	provideSecretMock(&kube, map[string]*corev1.Secret{
		pullSecretKey: {Data: map[string][]byte{
			".dockerconfigjson": nil,
		}},
		supportKey: {Data: map[string][]byte{
			"endpoint": []byte("test2"),
		}},
	})
	// 1. config update
	err := c.updateConfig(context.TODO())
	if err != nil {
		t.Fatalf("Unexpected error %s", err)
	}
	// Check if the event arrived at the channel
	if len(configCh) != 1 {
		t.Fatalf("Config channel has more/less than 1 event on a signal config change. len(configCh)==%d", len(configCh))
	}

	// Unsubscribe from config change
	closeFn()
	// Setup mock for 2. config update
	provideSecretMock(&kube, map[string]*corev1.Secret{
		pullSecretKey: {Data: map[string][]byte{
			".dockerconfigjson": nil,
		}},
		supportKey: {Data: map[string][]byte{
			"endpoint": []byte("test3"),
		}},
	})
	// 2. config update
	err = c.updateConfig(context.TODO())
	if err != nil {
		t.Fatalf("Unexpected error %s", err)
	}
	// Check if unsubscribe worked. ie: no new event on the channel
	if len(configCh) != 1 {
		t.Fatalf("The closing function failed to unsubscribe from the config change event. len(configCh)==%d", len(configCh))
	}
}

func provideSecretMock(kube kubernetes.Interface, secs map[string]*corev1.Secret) {
	kube.CoreV1().(*corefake.FakeCoreV1).AddReactor("get", "secrets",
		func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
			actionName := ""
			if getAction, ok := action.(clienttesting.GetAction); ok {
				actionName = getAction.GetName()
			}

			key := fmt.Sprintf("(%s) %s.%s", action.GetResource(), action.GetNamespace(), actionName)
			sv, ok := secs[key]

			if !ok {
				return false, nil, nil
			}
			return true, sv, nil
		})
}
