package integration

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// https://bugzilla.redhat.com/show_bug.cgi?id=1750665
// https://bugzilla.redhat.com/show_bug.cgi?id=1753755
func TestDefaultUploadFrequency(t *testing.T) {
	// delete any existing overriding secret
	err := clientset.CoreV1().Secrets("openshift-config").Delete("support", &metav1.DeleteOptions{})

	// if the secret is not found, continue, not a problem
	if err != nil && err.Error() != `secrets "support" not found` {
		t.Fatal(err.Error())
	}

	// restart insights-operator (delete pods)
	restartInsightsOperator(t)

	// check logs for "Gathering cluster info every 2h0m0s"
	checkPodsLogs(t, clientset, "Gathering cluster info every 2h0m0s")

	// verify it's possible to override it
	newSecret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "support",
			Namespace: "openshift-config",
		},
		Data: map[string][]byte{
			"interval": []byte("3m"),
		},
		Type: "Opaque",
	}

	_, err = clientset.CoreV1().Secrets("openshift-config").Create(&newSecret)
	if err != nil {
		t.Fatal(err.Error())
	}
	// restart insights-operator (delete pods)
	restartInsightsOperator(t)

	// check logs for "Gathering cluster info every 3m0s"
	checkPodsLogs(t, clientset, "Gathering cluster info every 3m0s")
}

// TestUnreachableHost checks if insights operator reports "degraded" after 5 unsuccessful upload attempts
// https://bugzilla.redhat.com/show_bug.cgi?id=1745973
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
	err := clientset.CoreV1().Secrets("openshift-config").Delete("support", &metav1.DeleteOptions{})

	// if the secret is not found, continue, not a problem
	if err != nil && err.Error() != `secrets "support" not found` {
		t.Fatal(err.Error())
	}
	_, err = clientset.CoreV1().Secrets("openshift-config").Create(&modifiedSecret)
	if err != nil {
		t.Fatal(err.Error())
	}
	// Restart insights-operator
	// oc delete pods --namespace=openshift-insights --all
	restartInsightsOperator(t)

	// Check the logs
	checkPodsLogs(t, clientset, "exceeded than threshold 5. Marking as degraded.")

	// Check the operator is degraded
	insightsDegraded := isOperatorDegraded(t, clusterOperatorInsights())
	if !insightsDegraded {
		t.Fatal("Insights is not degraded")
	}
	// Delete secret
	err = clientset.CoreV1().Secrets("openshift-config").Delete("support", &metav1.DeleteOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	// Check the operator is not degraded anymore
	errDegraded := wait.PollImmediate(1*time.Second, 20*time.Minute, func() (bool, error) {
		insightsDegraded := isOperatorDegraded(t, clusterOperatorInsights())
		if insightsDegraded {
			return false, nil
		}
		return true, nil
	})
	t.Log(errDegraded)
}

// https://bugzilla.redhat.com/show_bug.cgi?id=1782151
func TestClusterDefaultNodeSelector(t *testing.T) {
	// set default selctor of node-role.kubernetes.io/worker
	schedulers, err := configClient.Schedulers().List(metav1.ListOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	for _, scheduler := range schedulers.Items {
		if scheduler.ObjectMeta.Name == "cluster" {
			scheduler.Spec.DefaultNodeSelector = "node-role.kubernetes.io/worker="
			configClient.Schedulers().Update(&scheduler)
		}
	}

	// restart insights-operator (delete pods)
	restartInsightsOperator(t)

	// check the pod is scheduled
	newPods, err := clientset.CoreV1().Pods("openshift-insights").List(metav1.ListOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	for _, newPod := range newPods.Items {
		pod, err := clientset.CoreV1().Pods("openshift-insights").Get(newPod.Name, metav1.GetOptions{})
		if err != nil {
			panic(err.Error())
		}
		podConditions := pod.Status.Conditions
		for _, condition := range podConditions {
			if condition.Type == "PodScheduled" {
				if condition.Status != "True" {
					t.Log("Pod is not scheduled")
					t.Fatal(err.Error())
				}
			}
		}
		t.Log("Pod is scheduled")
	}
}
