package conditional

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

var testFiringAlertsMap = map[string][]AlertLabels{
	"test-alert-current": {
		{
			"namespace": "test-namespace",
			"pod":       "test-pod",
		},
	},
	"test-alert-previous": {
		{
			"namespace": "test-namespace",
			"pod":       "test-pod",
		},
	},
}

var testPod = corev1.Pod{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test-pod",
		Namespace: "test-namespace",
	},
	Status: corev1.PodStatus{
		Phase: corev1.PodRunning,
		ContainerStatuses: []corev1.ContainerStatus{
			{Name: "test-container"},
		},
	},
	Spec: corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "test-container"},
		},
	},
}

func Test_GatherLogsOfUnhealthyPods_Current(t *testing.T) {
	testGatherLogsOfUnhealthyPodsHelper(
		t,
		"test-alert-current",
		false,
		200,
		"conditional/namespaces/test-namespace/pods/test-pod/containers/test-container/logs/last-200-lines.log",
	)
}

func Test_GatherLogsOfUnhealthyPods_Previous(t *testing.T) {
	testGatherLogsOfUnhealthyPodsHelper(
		t,
		"test-alert-previous",
		true,
		20,
		"conditional/namespaces/test-namespace/pods/test-pod/containers/test-container/logs-previous/last-20-lines.log",
	)
}

func testGatherLogsOfUnhealthyPodsHelper(t *testing.T,
	alertName string,
	previous bool,
	tailLines int64,
	recordName string,
) {
	gatherer := Gatherer{firingAlerts: testFiringAlertsMap}
	ctx := context.Background()

	coreClient := kubefake.NewSimpleClientset().CoreV1()
	_, err := coreClient.Pods("test-namespace").Create(ctx, &testPod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("unable to create fake pod: %v", err)
	}

	rec, errs := gatherer.gatherLogsOfUnhealthyPods(ctx, coreClient, GatherLogsOfUnhealthyPodsParams{
		AlertName: alertName,
		TailLines: tailLines,
		Previous:  previous,
	})

	if len(errs) > 0 {
		t.Fatalf("unexpected error(s) returned by the log gathering function: %v", errs)
	}
	if len(rec) != 1 {
		t.Fatalf("unexpected number of records (expected: 1, actual: %d)", len(rec))
	}

	if rec[0].Name != recordName {
		t.Fatalf("unexpected 'Name' of the second log record: %q", rec[0].Name)
	}
}

func Test_GatherLogsOfUnhealthyPods_MissingNamespace(t *testing.T) {
	testGatherLogsOfUnhealthyPodsMissingHelper(t, map[string][]AlertLabels{
		"test-alert": {
			{
				"pod": "test-pod",
			},
		},
	}, "namespace")
}

func Test_GatherLogsOfUnhealthyPods_MissingPod(t *testing.T) {
	testGatherLogsOfUnhealthyPodsMissingHelper(t, map[string][]AlertLabels{
		"test-alert": {
			{
				"namespace": "test-namespace",
			},
		},
	}, "pod")
}

func testGatherLogsOfUnhealthyPodsMissingHelper(t *testing.T,
	firingAlertsMap map[string][]AlertLabels,
	missingLabel string,
) {
	gatherer := Gatherer{
		firingAlerts: firingAlertsMap,
	}

	ctx := context.Background()
	coreClient := kubefake.NewSimpleClientset().CoreV1()

	rec, errs := gatherer.gatherLogsOfUnhealthyPods(ctx, coreClient, GatherLogsOfUnhealthyPodsParams{
		AlertName: "test-alert",
		TailLines: 100,
		Previous:  false,
	})

	if len(rec) != 0 {
		t.Fatalf("unexpected number of records (expected: 0, actual: %d)", len(rec))
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, actual errors returned: %v", errs)
	}

	if errs[0].Error() != fmt.Sprintf("alert is missing '%s' label", missingLabel) {
		t.Fatalf("unexpected error message on missing '%s' alert label: %q", missingLabel, errs[0].Error())
	}
}
