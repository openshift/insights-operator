package clusterconfig

import (
	"context"
	"testing"

	"github.com/RedHatInsights/insights-operator-utils/tests/helpers"
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
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{}},
		metav1.CreateOptions{},
	)
	helpers.FailOnError(t, err)

	records, err := gatherOpenShiftAPIServerOperatorLastDayLogs(ctx, coreClient, []string{
		stringToSearch,
	})
	helpers.FailOnError(t, err)

	assert.Len(t, records, 1)
	assert.Equal(t, "logs/openshift-apiserver-operator", records[0].Name)
	assert.Equal(t, Raw{stringToSearch + "\n"}, records[0].Item)
}
