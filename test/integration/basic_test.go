package integration

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/insights-operator/pkg/controller/status"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// TestPullSecretExists makes sure that required pull-secret exists when tests are started
func TestPullSecretExists(t *testing.T) {
	pullSecret, err := clientset.CoreV1().Secrets(OpenShiftConfig).Get(PullSecret, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		t.Fatalf("The pull-secret should exist when cluster boots up: %s", err)
	}
	if err != nil {
		t.Fatalf("The pull-secret read failed: %s", err)
	}
	var (
		secretConfig []byte
		ok           bool
	)
	if secretConfig, ok = pullSecret.Data[".dockerconfigjson"]; !ok {
		t.Fatalf("The pull-secret didn't contain .dockerconfigjson key: %s", err)
	}
	obj := map[string]interface{}{}
	errUnmarshal := json.Unmarshal(secretConfig, &obj)
	if errUnmarshal != nil {
		t.Fatal(errUnmarshal.Error())
	}
	creds := obj["auths"].(map[string]interface{})
	if _, ok := creds[CloudOpenShiftCom]; !ok {
		t.Fatalf("not found secret for cloud.openshift.com")
	}
}

func TestIsIOHealthy(t *testing.T) {
	checkPodsLogs(t, clientset, `The operator is healthy`)
}

// Check if opt-in/opt-out works
func TestOptOutOptIn(t *testing.T) {
	// initially IO should be running
	errDisabled := wait.PollImmediate(1*time.Second, 30*time.Second, func() (bool, error) {
		insightsNotDisabled := !operatorStatus(t, clusterOperatorInsights(), status.OperatorDisabled, configv1.ConditionTrue)
		if insightsNotDisabled {
			return true, nil
		}
		return false, nil
	})
	if errDisabled != nil {
		t.Fatalf("The Cluster Operator wasn't enabled in the beginning")
	}

	// Backup pull secret from openshift-config namespace.
	// oc extract secret/pull-secret -n openshift-config --to=.
	pullSecret, err := clientset.CoreV1().Secrets(OpenShiftConfig).Get(PullSecret, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("The pull-secret read failed: %s", err)
	}
	// Backup support secret from openshift-config namespace.
	// oc extract secret/support -n openshift-config --to=.
	supportSecret, err := clientset.CoreV1().Secrets(OpenShiftConfig).Get(Support, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("The pull-secret read failed: %s", err)
	}
	resetSecrets := func() {
		err := forceUpdateSecret(OpenShiftConfig, PullSecret, pullSecret)
		if err != nil {
			t.Error(err)
		}
		err = forceUpdateSecret(OpenShiftConfig, Support, supportSecret)
		if err != nil {
			t.Error(err)
		}
	}
	// Edit the `.dockerconfigjson` file that was downloaded.
	// Remove the `cloud.openshift.com` JSON entry.
	var (
		secretConfig []byte
		ok           bool
	)
	newPullSecret := pullSecret.DeepCopy()
	if secretConfig, ok = newPullSecret.Data[".dockerconfigjson"]; !ok {
		t.Fatalf("The pull-secret didn't contain .dockerconfigjson key: %s", err)
	}
	obj := map[string]interface{}{}
	errUnmarshal := json.Unmarshal(secretConfig, &obj)
	if errUnmarshal != nil {
		t.Fatal(errUnmarshal.Error())
	}
	creds := obj["auths"].(map[string]interface{})

	delete(creds, CloudOpenShiftCom)

	modifiedConfig, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	newPullSecret.Data[".dockerconfigjson"] = modifiedConfig

	// Update the global cluster pull secret.
	// oc set data secret/pull-secret -n openshift-config --from-file=.dockerconfigjson=<pull-secret-location>
	_, err = clientset.CoreV1().Secrets(OpenShiftConfig).Update(newPullSecret)
	if err != nil {
		t.Fatalf("Cannot update pull-secret with secret without cloud.redhat.com secret. Error: %s ", err)
	}
	newSupportSecret := supportSecret.DeepCopy()
	// set the upload interval to 1 to speed up test
	newSupportSecret.Data["interval"] = []byte(fmt.Sprintf("%s", time.Minute*1))

	// Update the global cluster pull secret.
	// oc set data secret/support -n openshift-config --from-file=interval=<pull-secret-location>
	_, err = clientset.CoreV1().Secrets(OpenShiftConfig).Update(newSupportSecret)
	if err != nil {
		t.Fatalf("Cannot update support secret with secret with short interval. Error: %s ", err)
	}
	defer func() {
		resetSecrets()
	}()

	// Check the ClusterOperator status - Status is updated even while operator is only initializing.
	// Disabled status is written to logs only after initializing period
	restartInsightsOperator(t)

	// Wait for operator to become disabled because of removed pull-secret
	errDisabled = wait.PollImmediate(1*time.Second, 30*time.Second, func() (bool, error) {
		insightsDisabled := isOperatorDisabled(t, clusterOperatorInsights())
		if insightsDisabled {
			return true, nil
		}
		return false, nil
	})
	if errDisabled != nil {
		t.Fatalf("The Cluster Operator wasn't disabled after removing pull-secret")
	}
	// Return to original pull secret, so that IO would be enabled again
	err = forceUpdateSecret(OpenShiftConfig, PullSecret, pullSecret)
	if err != nil {
		t.Errorf("cannot return original pull-secret: %s", err)
	}

	// Check if reports are uploaded - Logs show that insights-operator is enabled and reports are uploaded
	restartInsightsOperator(t)
	errDisabled = wait.PollImmediate(1*time.Second, 3*time.Minute, func() (bool, error) {
		insightsNotDisabled := !operatorStatus(t, clusterOperatorInsights(), status.OperatorDisabled, configv1.ConditionTrue)
		if insightsNotDisabled {
			return true, nil
		}
		return false, nil
	})
	if errDisabled != nil {
		t.Fatalf("The Cluster Operator wasn't enabled after setting original pull-secret")
	}
	checkPodsLogs(t, clientset, "Successfully reported")
}
