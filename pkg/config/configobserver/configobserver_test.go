package configobserver

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clsetfake "k8s.io/client-go/kubernetes/fake"
	corefake "k8s.io/client-go/kubernetes/typed/core/v1/fake"
	"k8s.io/klog/v2"

	clienttesting "k8s.io/client-go/testing"
)

func Test_ConfigObserver_ChangeSupportConfig(t *testing.T) {
	var cases = []struct {
		name      string
		config    map[string]*corev1.Secret
		expConfig *config.Controller
		expErr    error
	}{
		{name: "interval too short",
			config: map[string]*corev1.Secret{
				pullSecretKey: {Data: map[string][]byte{
					".dockerconfigjson": nil,
				}},
				supportKey: {Data: map[string][]byte{
					"username": []byte("someone"),
					"password": []byte("secret"),
					"endpoint": []byte("http://po.rt"),
					"interval": []byte("1s"),
				}},
			},
			expErr: fmt.Errorf("insights secret interval must be a duration (1h, 10m) greater than or equal to ten seconds: too short"),
		},
		{name: "correct interval",
			config: map[string]*corev1.Secret{
				pullSecretKey: {Data: map[string][]byte{
					".dockerconfigjson": nil,
				}},
				supportKey: {Data: map[string][]byte{
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
		tt := tt
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
			c.mergeConfigLocked()
			err := c.retrieveToken(context.Background())
			if err == nil {
				err = c.retrieveConfig(context.Background())
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
				t.Fatalf("The test expected error doesn't match actual error.\nExpected: %s Actual: %s", tt.expErr, err)
			}
			if tt.expConfig != nil && !reflect.DeepEqual(tt.expConfig, c.config) {
				t.Fatalf("The test expected config doesn't match actual config.\nExpected: %v Actual: %v", tt.expConfig, c.config)
			}
		})
	}
}

const (
	pullSecretKey = "(/v1, Resource=secrets) openshift-config.pull-secret" //nolint: gosec
	supportKey    = "(/v1, Resource=secrets) openshift-config.support"
)

func provideSecretMock(kube kubernetes.Interface, secs map[string]*corev1.Secret) {
	kube.CoreV1().(*corefake.FakeCoreV1).Fake.AddReactor("get", "secrets",
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

type kubeClientResponder struct {
	clsetfake.Clientset
}

var _ kubernetes.Interface = (*kubeClientResponder)(nil)
