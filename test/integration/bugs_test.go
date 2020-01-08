package integration

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
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

// https://bugzilla.redhat.com/show_bug.cgi?id=1745973
// Check if insights operator reports "degraded" after 5 unsuccessful upload attempts
func TestUnreachableHost(t *testing.T) {
	// Replace the endpoint to some not valid url.
	// oc -n openshift-config create secret generic support --from-literal=endpoint=http://localhost --dry-run -o yaml | oc apply -f - -n openshift-config
	modifiedSecret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "support",
			Namespace: "openshift-config",
		},
		Data: map[string][]byte{
			"endpoint": []byte("http://localhost"),
			"interval": []byte("3m"), // for faster testing
		},
		Type: "Opaque",
	}
	// delete any existing overriding secret
	err := kubeClient.CoreV1().Secrets("openshift-config").Delete("support", &metav1.DeleteOptions{})

	// if the secret is not found, continue, not a problem
	if err != nil && err.Error() != `secrets "support" not found` {
		t.Fatal(err.Error())
	}
	_, err = kubeClient.CoreV1().Secrets("openshift-config").Create(&modifiedSecret)
	if err != nil {
		t.Fatal(err.Error())
	}
	// Restart insights-operator
	// oc delete pods --namespace=openshift-insights --all
	RestartInsightsOperator(t)

	// Check the logs
	CheckPodsLogs(t, kubeClient, "exceeded than threshold 5. Marking as degraded.")

	// Check the operator is degraded
	insightsDegraded := IsOperatorDegraded(t, COInsights(kubeClient))
	if !insightsDegraded {
		t.Fatal("Insights is not degraded")
	}
	// Delete secret
	kubeClient.CoreV1().Secrets("openshift-config").Delete("support", &metav1.DeleteOptions{})
	// Check the operator is not degraded anymore
	errDegraded := wait.PollImmediate(1*time.Second, 10*time.Minute, func() (bool, error) {
		insightsDegraded := IsOperatorDegraded(t, COInsights(kubeClient))
		if insightsDegraded {
			return false, nil
		}
		return true, nil
	})
	t.Log(errDegraded)
}
