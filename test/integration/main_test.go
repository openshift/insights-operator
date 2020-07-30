package integration

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	// namespace openshift-config
	OpenShiftConfig = "openshift-config"

	// secret support form namespace openshift-config
	Support = "support"

	// secret pull-secret from namespace openshift-config
	PullSecret = "pull-secret"

	// secret support pull token key is under auth
	CloudOpenShiftCom = "cloud.openshift.com"
)

var clientset = kubeClient()
var configClient = configV1Client()

func kubeconfig() (config *restclient.Config) {
	kubeconfig, ok := os.LookupEnv("KUBECONFIG") // variable is a path to the local kubeconfig
	if !ok {
		fmt.Printf("kubeconfig variable is not set\n")
	} else {
		fmt.Printf("KUBECONFIG=%s\n", kubeconfig)
	}
	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		fmt.Printf("%#v", err)
		os.Exit(1)
	}
	return config
}

func kubeClient() (result *kubernetes.Clientset) {
	clientset, err := kubernetes.NewForConfig(kubeconfig())
	if err != nil {
		panic(err.Error())
	}
	return clientset
}

func configV1Client() (result *configv1client.ConfigV1Client) {
	client, err := configv1client.NewForConfig(kubeconfig())
	if err != nil {
		panic(err.Error())
	}
	return client
}

func clusterOperatorInsights() *configv1.ClusterOperator {
	// get info about insights cluster operator
	operator, err := configClient.ClusterOperators().Get("insights", metav1.GetOptions{})
	if err != nil {
		panic(err.Error())
	}
	return operator
}

func isOperatorDegraded(t *testing.T, operator *configv1.ClusterOperator) bool {
	statusConditions := operator.Status.Conditions

	for _, condition := range statusConditions {
		if condition.Type == "Degraded" {
			if condition.Status == "True" {
				t.Logf("%s Operator is degraded ", time.Now())
				return true
			}
		}
	}
	t.Logf("%s Operator is not degraded", time.Now())
	return false
}

func isOperatorDisabled(t *testing.T, operator *configv1.ClusterOperator) bool {
	statusConditions := operator.Status.Conditions

	for _, condition := range statusConditions {
		if condition.Type == "Disabled" {
			if condition.Status == "True" {
				t.Log("Operator is Disabled")
				return true
			}
		}
	}
	t.Log("Operator is not disabled")
	return false
}

func operatorStatus(t *testing.T, operator *configv1.ClusterOperator, conditionType configv1.ClusterStatusConditionType, conditionStatus configv1.ConditionStatus) bool {
	statusConditions := operator.Status.Conditions

	for _, condition := range statusConditions {
		if condition.Type == conditionType {
			if condition.Status == conditionStatus {
				t.Logf("Operator has Condition Type %s with status %s", conditionType, conditionStatus)
				return true
			}
		}
	}
	t.Logf("Operator doesn't have Condition Type %s with status %s", conditionType, conditionStatus)
	return false
}

func restartInsightsOperator(t *testing.T) {
	// restart insights-operator (delete pods)
	pods, err := clientset.CoreV1().Pods("openshift-insights").List(metav1.ListOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	for _, pod := range pods.Items {
		clientset.CoreV1().Pods("openshift-insights").Delete(pod.Name, &metav1.DeleteOptions{})
		err := wait.PollImmediate(1*time.Second, 1*time.Minute, func() (bool, error) {
			_, err := clientset.CoreV1().Pods("openshift-insights").Get(pod.Name, metav1.GetOptions{})
			if err == nil {
				t.Logf("the pod is not yet deleted: %v\n", err)
				return false, nil
			}
			t.Log("the pod is deleted")
			return true, nil
		})
		t.Log(err)
	}

	// check new pods are created and running
	errPod := wait.PollImmediate(1*time.Second, 1*time.Minute, func() (bool, error) {
		newPods, _ := clientset.CoreV1().Pods("openshift-insights").List(metav1.ListOptions{})
		if len(newPods.Items) == 0 {
			t.Log("pods are not yet created")
			return false, nil
		}

		for _, newPod := range newPods.Items {
			pod, err := clientset.CoreV1().Pods("openshift-insights").Get(newPod.Name, metav1.GetOptions{})
			if err != nil {
				panic(err.Error())
			}
			if pod.Status.Phase != "Running" {
				return false, nil
			}
		}

		t.Log("the pods are created")
		return true, nil
	})
	t.Log(errPod)
}

func checkPodsLogs(t *testing.T, kubeClient *kubernetes.Clientset, message string) {
	newPods, err := kubeClient.CoreV1().Pods("openshift-insights").List(metav1.ListOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	for _, newPod := range newPods.Items {
		pod, err := kubeClient.CoreV1().Pods("openshift-insights").Get(newPod.Name, metav1.GetOptions{})
		if err != nil {
			panic(err.Error())
		}
		errLog := wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
			req := kubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{})
			podLogs, err := req.Stream()
			if err != nil {
				return false, nil
			}
			defer podLogs.Close()

			buf := new(bytes.Buffer)
			_, err = io.Copy(buf, podLogs)
			if err != nil {
				panic(err.Error())
			}
			log := buf.String()

			result := strings.Contains(log, message)
			if result == false {
				t.Logf("No %s in logs\n", message)
				t.Logf("Logs for verification: ****\n%s", log)
				return false, nil
			}

			t.Logf("%s found\n", message)
			return true, nil
		})
		if errLog != nil {
			t.Error(errLog)
		}
	}
}

func forceUpdateSecret(ns string, secretName string, secret *v1.Secret) error {
	latestSecret, err := clientset.CoreV1().Secrets(ns).Get(secretName, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("cannot read the original secret: %s", err)
	}
	if errors.IsNotFound(err) {
		// new objects shouldn't have resourceVersion set
		secret.SetResourceVersion("")
		_, err = clientset.CoreV1().Secrets(ns).Create(secret)
		if err != nil {
			return fmt.Errorf("cannot create the original secret: %s", err)
		}
		return nil
	}
	resourceVersion := latestSecret.GetResourceVersion()
	secret.SetUID(latestSecret.GetUID())
	secret.SetResourceVersion(resourceVersion) // need to update the version, otherwise operation is not permitted

	_, err = clientset.CoreV1().Secrets("openshift-config").Update(secret)
	if err != nil {
		return fmt.Errorf("Unable to update original secret: %s", err)
	}
	return nil
}

func TestMain(m *testing.M) {
	// check the operator is up
	err := waitForOperator(clientset)
	if err != nil {
		fmt.Println("failed waiting for operator to start")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func waitForOperator(kubeClient *kubernetes.Clientset) error {
	depClient := kubeClient.AppsV1().Deployments("openshift-insights")

	err := wait.PollImmediate(1*time.Second, 1*time.Minute, func() (bool, error) {
		_, err := depClient.Get("insights-operator", metav1.GetOptions{})
		if err != nil {
			fmt.Printf("error waiting for operator deployment to exist: %v\n", err)
			return false, nil
		}
		fmt.Println("found operator deployment")
		return true, nil
	})
	return err
}
