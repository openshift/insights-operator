package configobserver

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/utils"
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
		{name: "correct interval",
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
				t.Fatalf("The test expected error doesn't match actual error.\nExpected: %s Actual: %s", tt.expErr, err)
			}
			if tt.expConfig != nil && !reflect.DeepEqual(tt.expConfig, c.config) {
				t.Fatalf("The test expected config doesn't match actual config.\nExpected: %v Actual: %v", tt.expConfig, c.config)
			}
		})
	}

}

// ignore until resolved
func testChangeObserved(t *testing.T) {
	setIntervals := map[int]time.Duration{
		0: time.Duration(10 * time.Minute),
		1: time.Duration(1 * time.Minute),
		2: time.Duration(3 * time.Minute),
		3: time.Duration(4 * time.Minute),
	}

	klog.SetOutput(utils.NewTestLog(t).Writer())

	ctrl := config.Controller{}
	kube := kubeClientResponder{}
	// The initial values set in configobserver.New
	secs := map[string]*corev1.Secret{
		pullSecretKey: &corev1.Secret{Data: map[string][]byte{
			".dockerconfigjson": fakeDockerConfig(),
		}},
		supportKey: &corev1.Secret{Data: map[string][]byte{
			"username":  []byte("someone"),
			"password":  []byte("secret"),
			"endpoint":  []byte("http://po.rt"),
			intervalKey: []byte("10m"),
		}},
	}

	provideSecretMock(&kube, secs)
	// New reads first k8 configuration
	co := New(ctrl, &kube)
	// set some initial config because we are tracking changes only
	co.setConfigLocked(&config.Controller{})

	// observe changes every 50 ms
	co.checkPeriod = 50 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Watch for changes in configurations
	done := make(chan bool)
	go co.Start(ctx)
	changedC, _ := co.ConfigChanged()

	// Sets gather intervals in config to 3 and 4 minutes after 100ms elapses
	go func() {

		for i := range setIntervals {
			time.Sleep(100 * time.Millisecond)
			secs[supportKey].Data[intervalKey] = []byte(setIntervals[i].String())
		}
		// Give observer chance to catch the change
		time.Sleep(50 * time.Millisecond)
		done <- true
	}()

	actualIntervals := map[int]time.Duration{}
	actIntMu := sync.Mutex{}
	go func() {
		for {
			select {
			case <-ctx.Done():
				done <- true

			case <-changedC:
				actInt := co.Config().Interval

				actIntMu.Lock()
				actIntMu.Unlock()
				actualIntervals[len(actualIntervals)] = actInt
			}
		}
	}()
	<-done

	if !reflect.DeepEqual(setIntervals, actualIntervals) {
		t.Fatalf("the expected intervals didn't match actual intervals. \nExpected %v \nActual %v", setIntervals, actualIntervals)
	}
}

const (
	pullSecretKey = "(/v1, Resource=secrets) openshift-config.pull-secret"
	supportKey    = "(/v1, Resource=secrets) openshift-config.support"
	intervalKey   = "interval"
)

func fakeDockerConfig() []byte {
	d, _ := json.Marshal(
		serializedAuthMap{
			Auths: map[string]serializedAuth{
				"cloud.openshift.com": serializedAuth{Auth: ".."},
			},
		})
	return d
}

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
