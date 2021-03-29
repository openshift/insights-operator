package integration

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"k8s.io/api/certificates/v1beta1"
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

func genLatestArchiveCheckPattern(prettyName string, check func(*testing.T, string, []string, *regexp.Regexp) error, pattern string) func(t *testing.T) {
	return func(t *testing.T) {
		err := latestArchiveCheckFiles(t, prettyName, check, pattern)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func latestArchiveContainsConfigMaps(t *testing.T) {
	configMaps, _ := clientset.CoreV1().ConfigMaps("openshift-config").List(context.Background(), metav1.ListOptions{})
	if len(configMaps.Items) == 0 {
		t.Fatal("Nothing to test: no config maps in openshift-config namespace")
	}
	for _, configMap := range configMaps.Items {
		configMapPath := fmt.Sprintf("^config/configmaps/openshift-config/%s/.*$", configMap.Name)
		err := latestArchiveCheckFiles(t, "config map", matchingFileExists, configMapPath)
		if err != nil {
			t.Error(err)
		}
	}
}

func latestArchiveContainsNodes(t *testing.T) {
	Nodes, _ := clientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if len(Nodes.Items) == 0 {
		t.Fatal("Nothing to test: api doesn't return any nodes")
	}
	for _, node := range Nodes.Items {
		configMapPath := fmt.Sprintf("^config/node/%s\\.json$", node.Name)
		err := latestArchiveCheckFiles(t, "node", matchingFileExists, configMapPath)
		if err != nil {
			t.Error(err)
		}
	}
}

func TestArchiveContains(t *testing.T) {
	//https://bugzilla.redhat.com/show_bug.cgi?id=1825756
	t.Run("ConfigMaps", latestArchiveContainsConfigMaps)

	// not backported to 4.6 yet, uncomment when backported
	////https://bugzilla.redhat.com/show_bug.cgi?id=1885930
	//t.Run("ServiceAccounts",
	//	genLatestArchiveCheckPattern(
	//		"service accounts", matchingFileExists,
	//		`^config/serviceaccounts\.json$`))

	//https://bugzilla.redhat.com/show_bug.cgi?id=1834677
	t.Run("ImageRegistry",
		genLatestArchiveCheckPattern(
			"image registry", matchingFileExists,
			`^config/clusteroperator/imageregistry.operator.openshift.io/config/cluster\.json$`))

	//https://bugzilla.redhat.com/show_bug.cgi?id=1873101
	t.Run("SnapshotsCRD",
		genLatestArchiveCheckPattern(
			"snapshots CRD", matchingFileExists,
			`^config/crd/volumesnapshots\.snapshot\.storage\.k8s\.io\.json$`))

	defer ChangeReportTimeInterval(t, 1)()
	defer degradeOperatorMonitoring(t)()

	checker := LogChecker(t).Timeout(2 * time.Minute)
	checker.SinceNow().Search(`Recording events/openshift-monitoring`)
	checker.EnableSinceLastCheck().Search(`Wrote \d+ records to disk in \d+`)

	//https://bugzilla.redhat.com/show_bug.cgi?id=1868165
	t.Run("Nodes", latestArchiveContainsNodes)

	//https://bugzilla.redhat.com/show_bug.cgi?id=1881816
	t.Run("MachineSet",
		genLatestArchiveCheckPattern(
			"machine set", matchingFileExists,
			`^machinesets/.*\.json$`))

	//https://bugzilla.redhat.com/show_bug.cgi?id=1881905
	t.Run("PodDisruptionBudgets",
		genLatestArchiveCheckPattern(
			"pod disruption budgets", matchingFileExists,
			`^config/pdbs/.*\.json$`))

	t.Run("csr",
		genLatestArchiveCheckPattern(
			"csr", matchingFileExists,
			`^config/certificatesigningrequests/.*\.json$`))

	//https://bugzilla.redhat.com/show_bug.cgi?id=1879068
	t.Run("HostsSubnet",
		genLatestArchiveCheckPattern(
			"hosts subnet", matchingFileExists,
			`^config/hostsubnet/.*\.json$`))

	//https://bugzilla.redhat.com/show_bug.cgi?id=1838973
	t.Run("Logs",
		genLatestArchiveCheckPattern(
			"log", matchingFileExists,
			`^config/pod/openshift-monitoring/logs/.*\.log$`))

	//https://bugzilla.redhat.com/show_bug.cgi?id=1767719
	t.Run("Event",
		genLatestArchiveCheckPattern(
			"event", matchingFileExists,
			`^events/openshift-monitoring\.json$`))

	//https://bugzilla.redhat.com/show_bug.cgi?id=1840012
	t.Run("FileExtensions",
		genLatestArchiveCheckPattern(
			"extension of", allFilesMatch,
			knownFileSuffixesInsideArchiveRegex))
}

//https://bugzilla.redhat.com/show_bug.cgi?id=1835090
func TestCSRCollected(t *testing.T) {
	certificateRequest := []byte(`-----BEGIN CERTIFICATE REQUEST-----
MIIBYzCCAQgCAQAwMDEuMCwGA1UEAxMlbXktcG9kLm15LW5hbWVzcGFjZS5wb2Qu
Y2x1c3Rlci5sb2NhbDBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABKhgwkNZ1uTb
DKKwJAh9TmmpSXKlbogxqV8e0yjIa2tKHZScAiZwTw920d6PLIU984ivWYfez/gq
ATGDLWuX+Y2gdjB0BgkqhkiG9w0BCQ4xZzBlMGMGA1UdEQRcMFqCJW15LXN2Yy5t
eS1uYW1lc3BhY2Uuc3ZjLmNsdXN0ZXIubG9jYWyCJW15LXBvZC5teS1uYW1lc3Bh
Y2UucG9kLmNsdXN0ZXIubG9jYWyHBMAAAhiHBAoAIgIwCgYIKoZIzj0EAwIDSQAw
RgIhAIPCUx9FdzX1iDGxH9UgYJE07gfG+J3ObR31IHhmi+WwAiEAtzN35zYkXEaC
YLluQUO+Jy/PjOnMPw5+DeSX6asUgXE=
-----END CERTIFICATE REQUEST-----`)
	name := "my-svc.my-namespace"
	_, err := clientset.CertificatesV1beta1().CertificateSigningRequests().Create(context.Background(), &v1beta1.CertificateSigningRequest{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       v1beta1.CertificateSigningRequestSpec{Request: certificateRequest},
		Status:     v1beta1.CertificateSigningRequestStatus{},
	}, metav1.CreateOptions{})
	e(t, err, "Failed creating certificate signing request")
	defer func() {
		clientset.CertificatesV1beta1().CertificateSigningRequests().Delete(context.Background(), name, metav1.DeleteOptions{})
		restartInsightsOperator(t)
	}()
	defer ChangeReportTimeInterval(t, 1)()
	LogChecker(t).SinceNow().Search(`Uploaded report successfully in`)
	certificatePath := `^config/certificatesigningrequests/my-svc.my-namespace.json$`
	err = latestArchiveCheckFiles(t, "certificate request", matchingFileExists, certificatePath)
	e(t, err, "")
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
