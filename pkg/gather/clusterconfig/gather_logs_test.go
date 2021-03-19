package clusterconfig

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/openshift/insights-operator/pkg/utils/marshal"
)

func testGatherLogs(t *testing.T, regexSearch bool, stringToSearch string, shouldExist bool) {
	const testPodName = "test"
	const testLogFileName = "errors"

	coreClient := kubefake.NewSimpleClientset().CoreV1()
	ctx := context.Background()

	_, err := coreClient.Pods(testPodName).Create(
		ctx,
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testPodName,
				Namespace: testPodName,
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: testPodName},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: testPodName},
				},
			},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}

	records, err := gatherLogsFromContainers(
		ctx,
		coreClient,
		logContainersFilter{
			namespace: testPodName,
		},
		logMessagesFilter{
			messagesToSearch: []string{
				stringToSearch,
			},
			isRegexSearch: regexSearch,
			sinceSeconds:  86400,     // last day
			limitBytes:    1024 * 64, // maximum 64 kb of logs
		},
		testLogFileName,
	)
	if err != nil {
		t.Fatal(err)
	}

	if !shouldExist {
		assert.Len(t, records, 0)
		return
	}

	assert.Len(t, records, 1)
	assert.Equal(
		t,
		fmt.Sprintf("config/pod/%s/logs/%s/%s.log", testPodName, testPodName, testLogFileName),
		records[0].Name,
	)
	if regexSearch {
		assert.Regexp(t, stringToSearch, records[0].Item)
	} else {
		assert.Equal(t, marshal.Raw{Str: stringToSearch + "\n"}, records[0].Item)
	}
}

func Test_GatherLogs(t *testing.T) {
	t.Run("SubstringSearch_ShouldExist", func(t *testing.T) {
		testGatherLogs(t, false, "fake logs", true)
	})
	t.Run("SubstringSearch_ShouldNotExist", func(t *testing.T) {
		testGatherLogs(t, false, "The quick brown fox jumps over the lazy dog", false)
	})
	t.Run("SubstringSearch_ShouldNotExist", func(t *testing.T) {
		testGatherLogs(t, false, "f.*l", false)
	})

	t.Run("RegexSearch_ShouldExist", func(t *testing.T) {
		testGatherLogs(t, true, "f.*l", true)
	})
	t.Run("RegexSearch_ShouldNotExist", func(t *testing.T) {
		testGatherLogs(t, true, "[0-9]99", false)
	})
}
