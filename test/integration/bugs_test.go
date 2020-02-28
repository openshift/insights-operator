package integration

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

var (
	kubeClient = KubeClient()
)

// https://bugzilla.redhat.com/show_bug.cgi?id=1750665
// https://bugzilla.redhat.com/show_bug.cgi?id=1753755
func TestDefaultUploadFrequency(t *testing.T) {
	// delete any existing overriding secret
	err := kubeClient.CoreV1().Secrets("openshift-config").Delete("support", &metav1.DeleteOptions{})

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

	for _, pod := range pods.Items {
		kubeClient.CoreV1().Pods("openshift-insights").Delete(pod.Name, &metav1.DeleteOptions{})
		err := wait.PollImmediate(1*time.Second, 10*time.Minute, func() (bool, error) {
			_, err := kubeClient.CoreV1().Pods("openshift-insights").Get(pod.Name, metav1.GetOptions{})
			if err == nil {
				fmt.Printf("the pod is not yet deleted: %v\n", err)
				return false, nil
			}
			fmt.Println("the pod is deleted")
			return true, nil
		})
		fmt.Print(err)
	}

	// check new pods are created and running
	errPod := wait.PollImmediate(1*time.Second, 10*time.Minute, func() (bool, error) {
		newPods, _ := kubeClient.CoreV1().Pods("openshift-insights").List(metav1.ListOptions{})
		if len(newPods.Items) == 0 {
			fmt.Printf("pods are not yet created")
			return false, nil
		}

		for _, newPod := range newPods.Items {
			pod, err := kubeClient.CoreV1().Pods("openshift-insights").Get(newPod.Name, metav1.GetOptions{})
			if err != nil {
				panic(err.Error())
			}
			if pod.Status.Phase != "Running" {
				return false, nil
			}
		}

		fmt.Println("the pods are created")
		return true, nil
	})
	fmt.Print(errPod)

	// check logs for "Gathering cluster info every 2h0m0s"
	newPods, err := kubeClient.CoreV1().Pods("openshift-insights").List(metav1.ListOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	for _, newPod := range newPods.Items {
		pod, err := kubeClient.CoreV1().Pods("openshift-insights").Get(newPod.Name, metav1.GetOptions{})
		if err != nil {
			panic(err.Error())
		}
		req := kubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{})
		podLogs, err := req.Stream()
		if err != nil {
			panic(err.Error())
		}
		defer podLogs.Close()

		buf := new(bytes.Buffer)
		_, err = io.Copy(buf, podLogs)
		if err != nil {
			panic(err.Error())
		}
		log := buf.String()

		result := strings.Contains(log, "Gathering cluster info every 2h0m0s")
		if result == false {
			t.Error("Default upload frequency is not 2 hours")
		}
	}
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
