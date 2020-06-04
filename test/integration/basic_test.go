package integration

import (
	"encoding/json"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// Check if opt-in/opt-out works
func TestOptOutOptIn(t *testing.T) {
	// Backup pull secret from openshift-config namespace.
	// oc extract secret/pull-secret -n openshift-config --to=.
	pullSecret, err := clientset.CoreV1().Secrets("openshift-config").Get("pull-secret", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Edit the `.dockerconfigjson` file that was downloaded.
	// Remove the `cloud.openshift.com` JSON entry.
	newPullSecret := pullSecret.DeepCopy()
	secretConfig := newPullSecret.Data[".dockerconfigjson"]

	obj := map[string]interface{}{}
	errUnmarshal := json.Unmarshal([]byte(secretConfig), &obj)
	if errUnmarshal != nil {
		t.Fatal(errUnmarshal.Error())
	}
	creds := obj["auths"].(map[string]interface{})
	delete(creds, "cloud.openshift.com")

	modifiedConfig, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err.Error())
	}

	newPullSecret.Data[".dockerconfigjson"] = modifiedConfig

	// Update the global cluster pull secret.
	// oc set data secret/pull-secret -n openshift-config --from-file=.dockerconfigjson=<pull-secret-location>
	_, err = clientset.CoreV1().Secrets("openshift-config").Update(newPullSecret)
	if err != nil {
		t.Fatal(err.Error())
	}
	// Check the logs -  Logs contains the line "The operator is marked as disabled" and no reports are uploaded
	restartInsightsOperator(t)
	checkPodsLogs(t, clientset, "The operator is marked as disabled")

	// Upload backuped secret
	latestSecret, err := clientset.CoreV1().Secrets("openshift-config").Get("pull-secret", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	resourceVersion := latestSecret.GetResourceVersion()
	pullSecret.SetResourceVersion(resourceVersion) // need to update the version, otherwise operation is not permitted

	errConfig := wait.PollImmediate(5*time.Second, 10*time.Minute, func() (bool, error) {
		objs := map[string]interface{}{}
		errUnmarshals := json.Unmarshal([]byte(pullSecret.Data[".dockerconfigjson"]), &objs)
		if errUnmarshals != nil {
			t.Fatal(errUnmarshal.Error())
		}
		for key := range objs["auths"].(map[string]interface{}) {
			if key == "cloud.openshift.com" {
				return true, nil
			}
		}
		return false, nil
	})
	t.Log(errConfig)

	newSecret, err := clientset.CoreV1().Secrets("openshift-config").Update(pullSecret)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("%v\n", newSecret)

	// Check if reports are uploaded - Logs show that insights-operator is enabled and reports are uploaded
	restartInsightsOperator(t)
	errDisabled := wait.PollImmediate(1*time.Second, 20*time.Minute, func() (bool, error) {
		insightsDisabled := isOperatorDisabled(t, clusterOperatorInsights())
		if insightsDisabled {
			return false, nil
		}
		return true, nil
	})
	t.Log(errDisabled)
	checkPodsLogs(t, clientset, "Successfully reported")
}
