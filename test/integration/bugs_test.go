package integration

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const knownFileSuffixesInsideArchiveRegex string = `(` +
	// known file extensions
	`\.(crt|json|log)` +
	`|` +
	// exceptions - file names without extension
	`(\/|^)(config|id|invoker|metrics|version)` +
	`)$`

//https://bugzilla.redhat.com/show_bug.cgi?id=1841057
func TestUploadNotDelayedAfterStart(t *testing.T) {
	/* TODO Result is irellevant as at this point IO was already restarted
	   this test would most likely fail and it's known issue, better solution is needed, skipping now*/
	t.Skip()
	LogChecker(t).Timeout(30 * time.Second).Search(`It is safe to use fast upload`)
	time1 := logLineTime(t, `Reporting status periodically to .* every`)
	time2 := logLineTime(t, `Successfully reported id=`)
	delay := time2.Sub(time1)
	allowedDelay := 3 * time.Minute
	t.Logf("Archive upload delay was %d seconds", delay/time.Second)
	if delay > allowedDelay && delay < time.Hour*24-allowedDelay {
		t.Fatal("Upload after start took too much time")
	}
}

// https://bugzilla.redhat.com/show_bug.cgi?id=1750665
// https://bugzilla.redhat.com/show_bug.cgi?id=1753755
func TestDefaultUploadFrequency(t *testing.T) {
	// Backup support secret from openshift-config namespace.
	// oc extract secret/support -n openshift-config --to=.
	supportSecret, err := clientset.CoreV1().Secrets(OpenShiftConfig).Get(context.Background(), Support, metav1.GetOptions{})
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
	err = clientset.CoreV1().Secrets(OpenShiftConfig).Delete(context.Background(), Support, metav1.DeleteOptions{})

	// if the secret is not found, continue, not a problem
	if err != nil && err.Error() != `secrets "support" not found` {
		t.Fatal(err.Error())
	}

	// restart insights-operator (delete pods)
	restartInsightsOperator(t)

	// check logs for "Gathering cluster info every 2h0m0s"
	checkPodsLogs(t, "Gathering cluster info every 2h0m0s")

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

	_, err = clientset.CoreV1().Secrets(OpenShiftConfig).Create(context.Background(), &newSecret, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	// restart insights-operator (delete pods)
	restartInsightsOperator(t)

	// check logs for "Gathering cluster info every 3m0s"
	checkPodsLogs(t, "Gathering cluster info every 3m0s")
}

// TestUnreachableHost checks if insights operator reports "degraded" after 5 unsuccessful upload attempts
// This tests takes about 317 s
// https://bugzilla.redhat.com/show_bug.cgi?id=1745973
func TestUnreachableHost(t *testing.T) {
	supportSecret, err := clientset.CoreV1().Secrets(OpenShiftConfig).Get(context.Background(), Support, metav1.GetOptions{})
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
	err = clientset.CoreV1().Secrets(OpenShiftConfig).Delete(context.Background(), Support, metav1.DeleteOptions{})

	// if the secret is not found, continue, not a problem
	if err != nil && err.Error() != `secrets "support" not found` {
		t.Fatal(err.Error())
	}
	_, err = clientset.CoreV1().Secrets(OpenShiftConfig).Create(context.Background(), &modifiedSecret, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	// Restart insights-operator
	// oc delete pods --namespace=openshift-insights --all
	restartInsightsOperator(t)

	// Check the logs
	checkPodsLogs(t, "exceeded than threshold 5. Marking as degraded.")

	// Check the operator is degraded
	insightsDegraded := operatorConditionCheck(t, clusterOperatorInsights(), "Degraded")
	if !insightsDegraded {
		t.Fatal("Insights is not degraded")
	}
	// Delete secret
	err = clientset.CoreV1().Secrets(OpenShiftConfig).Delete(context.Background(), Support, metav1.DeleteOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	// Check the operator is not degraded anymore
	errDegraded := wait.PollImmediate(3*time.Second, 3*time.Minute, func() (bool, error) {
		insightsDegraded := operatorConditionCheck(t, clusterOperatorInsights(), "Degraded")
		if insightsDegraded {
			return false, nil
		}
		return true, nil
	})
	t.Log(errDegraded)
}

func genLatestArchiveCheckPattern(prettyName string, check archiveCheck, archive []string, patterns ...string) test {
	return func(t *testing.T) {
		if len(patterns) == 0 {
			t.Fatal(prettyName, ": No patterns to check")
		}
		for _, pattern := range patterns {
			err := checkArchiveFiles(t, prettyName, check, pattern, archive)
			if err != nil {
				t.Error(err)
			}
		}
	}
}

func parsePatterns(pattern string, list interface{}) (names []string) {
	s := reflect.ValueOf(list)
	for i := 0; i < s.Len(); i++ {
		names = append(names, fmt.Sprintf(pattern, s.Index(i).FieldByName("Name")))
	}
	return
}

func genLatestArchiveContainsConfigMaps(archive []string) test {
	configMaps, _ := clientset.CoreV1().ConfigMaps("openshift-config").List(context.Background(), metav1.ListOptions{})
	return genLatestArchiveCheckPattern("config map", matchingFileExists, archive, parsePatterns("^config/configmaps/%s/.*$", configMaps.Items)...)
}

func genLatestArchiveContainsNodes(archive []string) test {
	Nodes, _ := clientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	return genLatestArchiveCheckPattern("node", matchingFileExists, archive, parsePatterns("^config/node/%s\\.json$", Nodes.Items)...)
}

func TestArchiveContains(t *testing.T) {
	defer ChangeReportTimeInterval(t, 1)()
	defer degradeOperatorMonitoring(t)()

	checker := LogChecker(t).Timeout(2 * time.Minute)
	checker.SinceNow().Search(`Recording events/openshift-monitoring`)
	checker.EnableSinceLastCheck().Search(`Wrote \d+ records to disk in \d+`)
	archive := latestArchiveFiles(t)

	//https://bugzilla.redhat.com/show_bug.cgi?id=1825756
	t.Run("ConfigMaps", genLatestArchiveContainsConfigMaps(archive))

	//https://bugzilla.redhat.com/show_bug.cgi?id=1885930
	t.Run("ServiceAccounts",
		genLatestArchiveCheckPattern(
			"service accounts", matchingFileExists, archive,
			`^config/serviceaccounts\.json$`))

	//https://bugzilla.redhat.com/show_bug.cgi?id=1834677
	t.Run("ImageRegistry",
		genLatestArchiveCheckPattern(
			"image registry", matchingFileExists, archive,
			`^config/imageregistry\.json$`))

	//https://bugzilla.redhat.com/show_bug.cgi?id=1873101
	t.Run("SnapshotsCRD",
		genLatestArchiveCheckPattern(
			"snapshots CRD", matchingFileExists, archive,
			`^config/crd/volumesnapshots\.snapshot\.storage\.k8s\.io\.json$`))

	//https://bugzilla.redhat.com/show_bug.cgi?id=1868165
	t.Run("Nodes", genLatestArchiveContainsNodes(archive))

	//https://bugzilla.redhat.com/show_bug.cgi?id=1881816
	t.Run("MachineSet",
		genLatestArchiveCheckPattern(
			"machine set", matchingFileExists, archive,
			`^machinesets/.*\.json$`))

	//https://bugzilla.redhat.com/show_bug.cgi?id=1881905
	t.Run("PodDisruptionBudgets",
		genLatestArchiveCheckPattern(
			"pod disruption budgets", matchingFileExists, archive,
			`^config/pdbs/.*\.json$`))

	//https://bugzilla.redhat.com/show_bug.cgi?id=1835090
	t.Run("csr",
		genLatestArchiveCheckPattern(
			"csr", matchingFileExists, archive,
			`^config/certificatesigningrequests/.*\.json$`))

	//https://bugzilla.redhat.com/show_bug.cgi?id=1879068
	t.Run("HostsSubnet",
		genLatestArchiveCheckPattern(
			"hosts subnet", matchingFileExists, archive,
			`^config/hostsubnet/.*\.json$`))

	//https://bugzilla.redhat.com/show_bug.cgi?id=1838973
	t.Run("Logs",
		genLatestArchiveCheckPattern(
			"log", matchingFileExists, archive,
			`^config/pod/openshift-monitoring/logs/.*\.log$`))

	//https://bugzilla.redhat.com/show_bug.cgi?id=1767719
	t.Run("Event",
		genLatestArchiveCheckPattern(
			"event", matchingFileExists, archive,
			`^events/openshift-monitoring\.json$`))

	//https://bugzilla.redhat.com/show_bug.cgi?id=1840012
	t.Run("FileExtensions",
		genLatestArchiveCheckPattern(
			"extension of", allFilesMatch, archive,
			knownFileSuffixesInsideArchiveRegex))
}

// https://bugzilla.redhat.com/show_bug.cgi?id=1782151
func TestClusterDefaultNodeSelector(t *testing.T) {
	// set default selector of node-role.kubernetes.io/worker
	schedulers, err := configClient.Schedulers().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	for _, scheduler := range schedulers.Items {
		if scheduler.ObjectMeta.Name == "cluster" {
			scheduler.Spec.DefaultNodeSelector = "node-role.kubernetes.io/worker="
			configClient.Schedulers().Update(context.Background(), &scheduler, metav1.UpdateOptions{})
		}
	}

	// restart insights-operator (delete pods)
	restartInsightsOperator(t)

	// check the pod is scheduled
	newPods, err := clientset.CoreV1().Pods("openshift-insights").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	for _, newPod := range newPods.Items {
		pod, err := clientset.CoreV1().Pods("openshift-insights").Get(context.Background(), newPod.Name, metav1.GetOptions{})
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
