package conditional

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func Test_GatherLogsOfUnhealthyPods(t *testing.T) {
	gatherer := Gatherer{
		firingAlerts: map[string][]AlertLabels{
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
		},
	}

	ctx := context.Background()

	coreClient := kubefake.NewSimpleClientset().CoreV1()
	_, err := coreClient.Pods("test-namespace").Create(ctx,
		&corev1.Pod{
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
		},
		metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("unable to create fake pod: %v", err)
	}

	rec, errs := gatherer.gatherLogsOfUnhealthyPods(ctx, coreClient, GatherLogsOfUnhealthyPodsParams{
		AlertsCurrent:     []string{"test-alert-current"},
		AlertsPrevious:    []string{"test-alert-previous"},
		TailLinesCurrent:  100,
		TailLinesPrevious: 10,
	})

	if len(errs) > 0 {
		t.Fatalf("unexpected error(s) returned by the log gathering function: %v", errs)
	}
	if len(rec) != 2 {
		t.Fatalf("unexpected number of records (expected: 2, actual: %d)", len(rec))
	}

	// The order, in which the logs are gathered is fixed, so the current log
	// should be the first record and the previous log should be the second.
	if rec[0].Name != "conditional/unhealthy_logs/test-namespace/test-pod/test-container/current.log" {
		t.Fatalf("unexpected 'Name' of the first log record: %q", rec[0].Name)
	}
	if rec[1].Name != "conditional/unhealthy_logs/test-namespace/test-pod/test-container/previous.log" {
		t.Fatalf("unexpected 'Name' of the second log record: %q", rec[1].Name)
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
		AlertsCurrent:     []string{"test-alert"},
		AlertsPrevious:    []string{},
		TailLinesCurrent:  100,
		TailLinesPrevious: 10,
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
		AlertsCurrent:     []string{"test-alert"},
		AlertsPrevious:    []string{},
		TailLinesCurrent:  100,
		TailLinesPrevious: 10,
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
