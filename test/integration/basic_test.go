package integration

import (
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Check if opt-in/opt-out works
func TestOptOutOptIn(t *testing.T) {
	// Backup pull secret from openshift-config namespace.
	// oc extract secret/pull-secret -n openshift-config --to=.
	pullSecret, err := kubeClient.CoreV1().Secrets("openshift-config").Get("pull-secret", metav1.GetOptions{})
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
	_, err = kubeClient.CoreV1().Secrets("openshift-config").Update(newPullSecret)
	if err != nil {
		t.Fatal(err.Error())
	}
	// Check the logs -  Logs contains the line "The operator is marked as disabled" and no reports are uploaded
	RestartInsightsOperator(t)
	CheckPodsLogs(t, kubeClient, "The operator is marked as disabled")

	// Upload backuped secret
	latestSecret, err := kubeClient.CoreV1().Secrets("openshift-config").Get("pull-secret", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	resourceVersion := latestSecret.GetResourceVersion()
	pullSecret.SetResourceVersion(resourceVersion) // need to update the version, otherwise operation is not permitted

	_, err = kubeClient.CoreV1().Secrets("openshift-config").Update(pullSecret)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Check if reports are uploaded - Logs show that insights-operator is enabled and reports are uploaded
	RestartInsightsOperator(t)
	CheckPodsLogs(t, kubeClient, "Successfully reported")
}
