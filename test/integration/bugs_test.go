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
func TestDefaultUploadFrequency(t *testing.T) {
	// delete any existing overriding secret
	err := kubeClient.CoreV1().Secrets("openshift-config").Delete("support", &metav1.DeleteOptions{})

	// if the secret is not found, continue, not a problem
	if err != nil && err.Error() != `secrets "support" not found` {
		t.Fatal(err.Error())
	}

	// restart insights-operator (delete pods)
	pods, err := kubeClient.CoreV1().Pods("openshift-insights").List(metav1.ListOptions{})
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
