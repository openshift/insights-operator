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

func TestGatherOpenShiftAPIServerOperatorLogs(t *testing.T) {
	const testPodNamespace = "openshift-apiserver-operator"
	// there's no way to specify logs fake pod will have, so we can only search for a hardcoded string "fake logs"
	const stringToSearch = "fake logs"

	coreClient := kubefake.NewSimpleClientset().CoreV1()
	ctx := context.Background()

	_, err := coreClient.Pods(testPodNamespace).Create(
		ctx,
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name: testPodNamespace,
		}},
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}

	records, err := gatherOpenShiftAPIServerOperatorLastDayLogs(ctx, coreClient, []string{
		stringToSearch,
	})
	if err != nil {
		t.Fatal(err)
	}

	assert.Len(t, records, 1)
	assert.Equal(
		t,
		fmt.Sprintf("config/pod/%s/logs/%s/errors.log", testPodNamespace, testPodNamespace),
		records[0].Name,
	)
	assert.Equal(t, Raw{stringToSearch + "\n"}, records[0].Item)
}
