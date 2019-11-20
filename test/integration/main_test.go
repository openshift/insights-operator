package integration

import (
	"fmt"
	"os"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func KubeClient() (result *kubernetes.Clientset) {
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

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	return kubeClient
}

func TestMain(m *testing.M) {
	kubeClient = KubeClient()
	// check the operator is up
	err := waitForOperator(kubeClient)
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
