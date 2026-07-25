package util

import (
	"math/rand"
	"os"
	"sync"
	"time"

	insightsclientset "github.com/openshift/client-go/insights/clientset/versioned"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	once           sync.Once
	kubeClient     kubernetes.Interface
	insightsClient insightsclientset.Interface
	restConfig     *rest.Config
)

func initClients() {
	once.Do(func() {
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			panic("KUBECONFIG environment variable must be set")
		}

		config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			panic(err)
		}
		restConfig = config

		kubeClient, err = kubernetes.NewForConfig(config)
		if err != nil {
			panic(err)
		}

		insightsClient, err = insightsclientset.NewForConfig(config)
		if err != nil {
			panic(err)
		}
	})
}

// GetKubeClient returns the Kubernetes client
func GetKubeClient() kubernetes.Interface {
	initClients()
	return kubeClient
}

// GetInsightsClient returns the Insights API client
func GetInsightsClient() insightsclientset.Interface {
	initClients()
	return insightsClient
}

// GetRestConfig returns the REST config
func GetRestConfig() *rest.Config {
	initClients()
	return restConfig
}

// RandomSuffix generates a random suffix for unique naming
func RandomSuffix() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, 8)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}
