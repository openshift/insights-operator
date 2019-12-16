package integration

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// https://bugzilla.redhat.com/show_bug.cgi?id=1750665
func TestDefaultUploadFrequency(t *testing.T) {
	// delete any existing overriding secret
	err := kubeClient.CoreV1().Secrets("openshift-config").Delete("support", &metav1.DeleteOptions{})

	// if the secret is not found, continue, not a problem
	if err != nil && err.Error() != `secrets "support" not found` {
		t.Fatal(err.Error())
	}

	// restart insights-operator (delete pods)
	RestartInsightsOperator(t)

	// check logs for "Gathering cluster info every 2h0m0s"
	CheckPodsLogs(t, kubeClient, "Gathering cluster info every 2h0m0s")
}
