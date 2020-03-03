package configobserver

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"testing"
	"time"

	"code.soquee.net/testlog"
	"github.com/openshift/insights-operator/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clsetfake "k8s.io/client-go/kubernetes/fake"
	corefake "k8s.io/client-go/kubernetes/typed/core/v1/fake"
	"k8s.io/klog"

	clienttesting "k8s.io/client-go/testing"
)

func TestChangeSupportConfig(t *testing.T) {
	var cases = []struct {
		name      string
		config    map[string]*corev1.Secret
		expConfig *config.Controller
		expErr    error
	}{
		{name: "interval too short",
			config: map[string]*corev1.Secret{
				pullSecretKey: &corev1.Secret{Data: map[string][]byte{
					".dockerconfigjson": nil,
				}},
				supportKey: &corev1.Secret{Data: map[string][]byte{
					"username": []byte("someone"),
					"password": []byte("secret"),
					"endpoint": []byte("http://po.rt"),
					"interval": []byte("1s"),
				}},
			},
			expErr: fmt.Errorf("insights secret interval must be a duration (1h, 10m) greater than or equal to one minute: too short"),
		},
		{name: "interval too short",
			config: map[string]*corev1.Secret{
				pullSecretKey: &corev1.Secret{Data: map[string][]byte{
					".dockerconfigjson": nil,
				}},
				supportKey: &corev1.Secret{Data: map[string][]byte{
					"interval": []byte("1m"),
				}},
			},
			expConfig: &config.Controller{
				Interval: 1 * time.Minute,
			},
			expErr: nil,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {

			// log klog to test
			klog.SetOutput(testlog.New(t).Writer())

			ctrl := config.Controller{}
			kube := kubeClientResponder{}
			secs := tt.config
			// setup mock responses for secretes by secret name
			kube.CoreV1().(*corefake.FakeCoreV1).Fake.AddReactor("get", "secrets",
				func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
					actionName := ""
					if getAction, ok := action.(clienttesting.GetAction); ok {
						actionName = getAction.GetName()
					}

					//log.Printf("namespace %s resource: %s verb %s Name %s", action.GetNamespace(), action.GetResource(), action.GetVerb(), actionName)
					key := fmt.Sprintf("(%s) %s.%s", action.GetResource(), action.GetNamespace(), actionName)
					sv, ok := secs[key]
					if !ok {
						return false, nil, nil
					}
					return true, sv, nil
				})
			// imitates New function
			c := &Controller{
				kubeClient:    &kube,
				defaultConfig: ctrl,
			}
			c.mergeConfigLocked()
			err := c.retrieveToken()
			if err == nil {
				err = c.retrieveConfig()
			}
			expErrS := ""
			if tt.expErr != nil {
				expErrS = tt.expErr.Error()
			}
			errS := ""
			if err != nil {
				errS = err.Error()
			}
			if expErrS != errS {
				t.Fatalf("The test expected error doesn't fit actual error.\nExpected: %s Actual: %s", tt.expErr, err)
			}
			if tt.expConfig != nil && !reflect.DeepEqual(tt.expConfig, c.config) {
				t.Fatalf("The test expected config doesn't fit actual config.\nExpected: %v Actual: %v", tt.expConfig, c.config)
			}
		})
	}

}

var (
	pullSecretKey = "(/v1, Resource=secrets) openshift-config.pull-secret"
	supportKey    = "(/v1, Resource=secrets) openshift-config.support"
	intervalKey   = "interval"
)

func mustMarshal(v interface{}) []byte {
	bt, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return bt
}

func mustLoad(filename string) []byte {
	f, err := os.Open(filename)
	if err != nil {
		panic(fmt.Errorf("test failed to load data: %v", err))
	}
	defer f.Close()
	bts, err := ioutil.ReadAll(f)
	if err != nil {
		panic(fmt.Errorf("test failed to read data: %v", err))
	}
	return bts
}

type kubeClientResponder struct {
	clsetfake.Clientset
}

var _ kubernetes.Interface = (*kubeClientResponder)(nil)
