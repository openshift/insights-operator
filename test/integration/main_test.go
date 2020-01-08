package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var clientset = kubeClient()

func kubeClient() (result *kubernetes.Clientset) {
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

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	return clientset
}

func clusterOperatorInsights(k *kubernetes.Clientset) map[string]interface{} {
	// get info about insights cluster operator
	data, err := k.RESTClient().Get().AbsPath("/apis/config.openshift.io/v1/clusteroperators/insights").DoRaw()
	obj := map[string]interface{}{}
	err = json.Unmarshal(data, &obj)
	if err != nil {
		panic(err.Error())
	}
	return obj
}

func isOperatorDegraded(t *testing.T, config map[string]interface{}) bool {
	status := config["status"].(map[string]interface{})
	statusConditions := status["conditions"].([]interface{})

	for _, condition := range statusConditions {
		c := condition.(map[string]interface{})
		if c["type"].(string) == "Degraded" || c["type"].(string) == "UploadDegraded" {
			if c["status"].(string) != "True" {
				t.Log("Insights is not degraded")
				return false
			}
		}
	}
	return true
}

func restartInsightsOperator(t *testing.T) {
	// restart insights-operator (delete pods)
	pods, err := clientset.CoreV1().Pods("openshift-insights").List(metav1.ListOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	for _, pod := range pods.Items {
		clientset.CoreV1().Pods("openshift-insights").Delete(pod.Name, &metav1.DeleteOptions{})
		err := wait.PollImmediate(1*time.Second, 10*time.Minute, func() (bool, error) {
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
	errPod := wait.PollImmediate(1*time.Second, 10*time.Minute, func() (bool, error) {
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

		errLog := wait.PollImmediate(5*time.Second, 15*time.Minute, func() (bool, error) {
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

	err := wait.PollImmediate(1*time.Second, 10*time.Minute, func() (bool, error) {
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
