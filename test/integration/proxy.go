package integration

import (
	"context"
	"fmt"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v12 "k8s.io/client-go/kubernetes/typed/core/v1"
	"testing"
	"time"
)

type TinyProxy struct {
	pod        *v1.Pod
	name       string
	client     v12.PodInterface
	IP         string
	PORT       string
	address    string
	status     string
	ready      bool
	LogChecker *LogCheck
	t          *testing.T
}

func (proxy *TinyProxy) create(t *testing.T, clientset *kubernetes.Clientset) error {
	proxy.t = t
	content, err := ioutil.ReadFile("resources_situational/tiny.yaml")
	if err != nil {
		return fmt.Errorf("failed to load local pod file: %e", err)
	}
	decoder := serializer.NewCodecFactory(scheme.Scheme).UniversalDecoder()
	pod := &corev1.Pod{}
	err = runtime.DecodeInto(decoder, content, pod)
	if err != nil {
		return fmt.Errorf("failed to decode local pod file: %e", err)
	}
	namespace := corev1.NamespaceDefault
	proxy.client = clientset.CoreV1().Pods(namespace)
	result, err := proxy.client.Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create pod: %e", err)
	}
	proxy.name = result.Name
	proxy.LogChecker = logChecker(t, clientset).Namespace(namespace).PodName(proxy.name)
	t.Logf("created pod %q.\n", proxy.name)
	return proxy.pullInfo()
}

func (proxy *TinyProxy) pullInfo() error {
	pod, err := proxy.client.Get(context.Background(), proxy.name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("pod %s does not exist: %e", proxy.name, err)
	}
	proxy.pod = pod
	proxy.status = string(pod.Status.Phase)
	proxy.ready = proxy.status == "Running"
	if !proxy.ready {
		return nil
	}
	IP := pod.Status.PodIPs[0].IP
	PORT := pod.Spec.Containers[0].Ports[0].ContainerPort
	proxy.address = fmt.Sprintf("%s:%d", IP, PORT)
	return nil
}

func (proxy *TinyProxy) waitUntilReady() {
	wait.PollImmediate(1*time.Second, 1*time.Minute, func() (bool, error) {
		proxy.pullInfo()
		proxy.t.Logf("%s status is %s", proxy.name, proxy.status)
		return proxy.ready, nil
	})
}

func (proxy *TinyProxy) delete() {
	err := proxy.client.Delete(context.Background(), proxy.name, metav1.DeleteOptions{})
	t := proxy.t
	t.Logf("deleting pod %s...", proxy.name)
	if err != nil {
		t.Logf("failed to delete pod " + proxy.name)
	}
}
