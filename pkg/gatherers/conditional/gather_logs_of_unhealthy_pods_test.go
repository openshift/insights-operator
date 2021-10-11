package conditional

import (
	"context"
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
	gatherer := Gatherer{firingAlerts: testFiringAlertsMap}
	ctx := context.Background()

	coreClient := kubefake.NewSimpleClientset().CoreV1()
	_, err := coreClient.Pods("test-namespace").Create(ctx, &testPod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("unable to create fake pod: %v", err)
	}

	rec, errs := gatherer.gatherLogsOfUnhealthyPods(ctx, coreClient, GatherLogsOfUnhealthyPodsParams{
		AlertName: "test-alert-current",
		TailLines: 100,
		Previous:  false,
	})

	if len(errs) > 0 {
		t.Fatalf("unexpected error(s) returned by the log gathering function: %v", errs)
	}
	if len(rec) != 1 {
		t.Fatalf("unexpected number of records (expected: 1, actual: %d)", len(rec))
	}

	if rec[0].Name != "conditional/unhealthy_logs/test-namespace/test-pod/test-container/current.log" {
		t.Fatalf("unexpected 'Name' of the first log record: %q", rec[0].Name)
	}
}

func Test_GatherLogsOfUnhealthyPods_Previous(t *testing.T) {
	gatherer := Gatherer{firingAlerts: testFiringAlertsMap}
	ctx := context.Background()

	coreClient := kubefake.NewSimpleClientset().CoreV1()
	_, err := coreClient.Pods("test-namespace").Create(ctx, &testPod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("unable to create fake pod: %v", err)
	}

	rec, errs := gatherer.gatherLogsOfUnhealthyPods(ctx, coreClient, GatherLogsOfUnhealthyPodsParams{
		AlertName: "test-alert-previous",
		TailLines: 100,
		Previous:  true,
	})

	if len(errs) > 0 {
		t.Fatalf("unexpected error(s) returned by the log gathering function: %v", errs)
	}
	if len(rec) != 1 {
		t.Fatalf("unexpected number of records (expected: 1, actual: %d)", len(rec))
	}

	if rec[0].Name != "conditional/unhealthy_logs/test-namespace/test-pod/test-container/previous.log" {
		t.Fatalf("unexpected 'Name' of the second log record: %q", rec[0].Name)
	}
}

func Test_GatherLogsOfUnhealthyPods_MissingNamespace(t *testing.T) {
	gatherer := Gatherer{
		firingAlerts: map[string][]AlertLabels{
			"test-alert": {
				{
					"pod": "test-pod",
				},
			},
		},
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

	if errs[0].Error() != "alert is missing 'namespace' label" {
		t.Fatalf("unexpected error message on missing 'namespace' alert label: %q", errs[0].Error())
	}
}

func Test_GatherLogsOfUnhealthyPods_MissingPod(t *testing.T) {
	gatherer := Gatherer{
		firingAlerts: map[string][]AlertLabels{
			"test-alert": {
				{
					"namespace": "test-namespace",
				},
			},
		},
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

	if errs[0].Error() != "alert is missing 'pod' label" {
		t.Fatalf("unexpected error message on missing 'pod' alert label: %q", errs[0].Error())
	}
}
