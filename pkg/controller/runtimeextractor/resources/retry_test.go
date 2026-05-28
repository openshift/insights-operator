package resources

import (
	"context"
	"testing"

	"github.com/openshift/library-go/pkg/operator/events"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/clock"
)

// Test_applyDaemonSet_RetryOnConflict verifies that DaemonSet apply retries on conflict errors
func Test_applyDaemonSet_RetryOnConflict(t *testing.T) {
	ctx := context.Background()

	// Create initial DaemonSet
	initialDS := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "insights-runtime-extractor",
			Namespace:       "openshift-insights",
			ResourceVersion: "1",
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "extractor", Image: "old-image:v1"},
						{Name: "exporter", Image: "old-image:v1"},
						{Name: "kube-rbac-proxy", Image: "old-image:v1"},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientset(initialDS)
	recorder := events.NewInMemoryRecorder("test", clock.RealClock{})

	// Track update attempts
	updateAttempts := 0

	// Add reactor to simulate conflict on first attempt, then succeed
	fakeClient.PrependReactor("update", "daemonsets", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		updateAttempts++
		if updateAttempts == 1 {
			// First attempt: return conflict error
			return true, nil, apierrors.NewConflict(
				appsv1.Resource("daemonsets"),
				"insights-runtime-extractor",
				nil,
			)
		}
		// Second attempt: succeed with default behavior
		return false, nil, nil
	})

	rm := NewResourceManager(
		fakeClient.AppsV1(),
		recorder,
	)

	// This should succeed after retry
	_, err := rm.applyDaemonSet(ctx)
	if err != nil {
		t.Fatalf("Expected applyDaemonSet to succeed after retry, got error: %v", err)
	}

	// Verify it retried (should have 2 attempts: 1 conflict + 1 success)
	if updateAttempts < 2 {
		t.Errorf("Expected at least 2 update attempts (conflict + retry), got %d", updateAttempts)
	}
}
