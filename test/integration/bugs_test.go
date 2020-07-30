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
	// Backup support secret from openshift-config namespace.
	// oc extract secret/support -n openshift-config --to=.
	supportSecret, err := clientset.CoreV1().Secrets(OpenShiftConfig).Get(Support, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("The support secret read failed: %s", err)
	}
	resetSecrets := func() {
		err = forceUpdateSecret(OpenShiftConfig, Support, supportSecret)
		if err != nil {
			t.Error(err)
		}
	}
	defer func() {
		resetSecrets()
	}()
	// delete any existing overriding secret
	err = clientset.CoreV1().Secrets(OpenShiftConfig).Delete(Support, &metav1.DeleteOptions{})

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
			Name:      Support,
			Namespace: OpenShiftConfig,
		},
		Data: map[string][]byte{
			"interval": []byte("3m"),
		},
		Type: "Opaque",
	}

	_, err = clientset.CoreV1().Secrets(OpenShiftConfig).Create(&newSecret)
	if err != nil {
		t.Fatal(err.Error())
	}
	// restart insights-operator (delete pods)
	restartInsightsOperator(t)

	// check logs for "Gathering cluster info every 3m0s"
	checkPodsLogs(t, clientset, "Gathering cluster info every 3m0s")
}

// TestUnreachableHost checks if insights operator reports "degraded" after 5 unsuccessful upload attempts
// This tests takes about 317 s
// https://bugzilla.redhat.com/show_bug.cgi?id=1745973
func TestUnreachableHost(t *testing.T) {
	supportSecret, err := clientset.CoreV1().Secrets(OpenShiftConfig).Get(Support, metav1.GetOptions{})
	e(t, err, "The support secret read failed:")
	resetSecrets := func() {
		err = forceUpdateSecret(OpenShiftConfig, Support, supportSecret)
		if err != nil {
			t.Error(err)
		}
	}
	defer func() {
		resetSecrets()
	}()
	// Replace the endpoint to some not valid url.
	// oc -n openshift-config create secret generic support --from-literal=endpoint=http://localhost --dry-run -o yaml | oc apply -f - -n openshift-config
	modifiedSecret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      Support,
			Namespace: OpenShiftConfig,
		},
		Data: map[string][]byte{
			"endpoint": []byte("http://localhost"),
			"interval": []byte("1m"), // for faster testing
		},
		Type: "Opaque",
	}
	// delete any existing overriding secret
	err = clientset.CoreV1().Secrets(OpenShiftConfig).Delete(Support, &metav1.DeleteOptions{})

	// if the secret is not found, continue, not a problem
	if err!=nil && err.Error() != `secrets "support" not found` {
		e(t, err)
	}
	_, err = clientset.CoreV1().Secrets(OpenShiftConfig).Create(&modifiedSecret)
	e(t, err)
	// Restart insights-operator
	// oc delete pods --namespace=openshift-insights --all
	restartInsightsOperator(t)

	// Check the logs
	checkPodsLogs(t, clientset, "exceeded than threshold 5. Marking as degraded.")

	// Check the operator is degraded
	insightsDegraded := isOperatorDegraded(t, clusterOperator(t, "insights"))
	if !insightsDegraded {
		t.Fatal("Insights is not degraded")
	}
	// Delete secret
	err = clientset.CoreV1().Secrets(OpenShiftConfig).Delete(Support, &metav1.DeleteOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	// Check the operator is not degraded anymore
	errDegraded := wait.PollImmediate(3*time.Second, 3*time.Minute, func() (bool, error) {
		insightsDegraded := isOperatorDegraded(t, clusterOperator(t, "insights"))
		if insightsDegraded {
			return false, nil
		}
		return true, nil
	})
	t.Log(errDegraded)
}

//https://bugzilla.redhat.com/show_bug.cgi?id=1838973
func TestPodLogsCollected(t *testing.T) {
	defer ChangeReportTimeInterval(t, 1)()
	pod := findPod(t, clientset, "openshift-monitoring", "cluster-monitoring-operator")
	defer degradeOperator(t, clientset, pod)()
	checkPodsLogs(t, clientset, `Wrote \d+ records to disk in \d+`, true)
	if !LatestArchiveContainsPodLogs(t, clientset, pod) {
		t.Fatal("There are no logs!")
	}
}

// https://bugzilla.redhat.com/show_bug.cgi?id=1782151
func TestClusterDefaultNodeSelector(t *testing.T) {
	// set default selector of node-role.kubernetes.io/worker
	schedulers, err := configV1Client(t).Schedulers().List(metav1.ListOptions{})
	e(t, err)
	for _, scheduler := range schedulers.Items {
		if scheduler.ObjectMeta.Name == "cluster" {
			scheduler.Spec.DefaultNodeSelector = "node-role.kubernetes.io/worker="
			configV1Client(t).Schedulers().Update(&scheduler)
		}
	}

	// restart insights-operator (delete pods)
	restartInsightsOperator(t)

	// check the pod is scheduled
	newPods, err := clientset.CoreV1().Pods("openshift-insights").List(metav1.ListOptions{})
	e(t, err)

	for _, newPod := range newPods.Items {
		pod, err := clientset.CoreV1().Pods("openshift-insights").Get(newPod.Name, metav1.GetOptions{})
		e(t, err)
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
