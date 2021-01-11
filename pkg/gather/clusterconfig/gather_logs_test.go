package clusterconfig

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestGatherLogs(t *testing.T) {
	const testPodNamespace = "pod-namespace"
	const testLogFileName = "errors"
	// there's no way to specify logs fake pod will have, so we can only search for a hardcoded string "fake logs"
	const stringToSearch = "fake logs"

	coreClient := kubefake.NewSimpleClientset().CoreV1()
	ctx := context.Background()

	_, err := coreClient.Pods(testPodNamespace).Create(
		ctx,
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testPodNamespace,
				Namespace: testPodNamespace,
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: testPodNamespace},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: testPodNamespace},
				},
			},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}

	records, err := gatherLogsFromPodsInNamespace(
		ctx,
		coreClient,
		testPodNamespace,
		[]string{
			stringToSearch,
		},
		86400,   // last day
		1024*64, // maximum 64 kb of logs
		testLogFileName,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}

	assert.Len(t, records, 1)
	assert.Equal(
		t,
		fmt.Sprintf("config/pod/%s/logs/%s/%s.log", testPodNamespace, testPodNamespace, testLogFileName),
		records[0].Name,
	)
	assert.Equal(t, Raw{stringToSearch + "\n"}, records[0].Item)
}
