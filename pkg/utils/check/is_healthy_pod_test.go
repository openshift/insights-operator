package check

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_IsHealthyPod(t *testing.T) {
	now := time.Now()
	oneMinuteAgo := metav1.NewTime(now.Add(-1 * time.Minute))
	threeMinutesAgo := metav1.NewTime(now.Add(-3 * time.Minute))

	tests := []struct {
		name     string
		pod      *corev1.Pod
		now      time.Time
		expected bool
	}{
		{
			name: "healthy running pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: oneMinuteAgo,
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							RestartCount: 0,
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{},
							},
						},
					},
				},
			},
			now:      now,
			expected: true,
		},
		{
			name: "pending pod less than 2 minutes old",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: oneMinuteAgo,
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
				},
			},
			now:      now,
			expected: true,
		},
		{
			name: "pending pod more than 2 minutes old",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: threeMinutesAgo,
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
				},
			},
			now:      now,
			expected: false,
		},
		{
			name: "container with non-zero exit code in current state",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: oneMinuteAgo,
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodFailed,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							RestartCount: 0,
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode: 1,
								},
							},
						},
					},
				},
			},
			now:      now,
			expected: false,
		},
		{
			name: "container with restart count greater than 0",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: oneMinuteAgo,
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							RestartCount: 1,
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{},
							},
						},
					},
				},
			},
			now:      now,
			expected: false,
		},
		{
			name: "init container with non-zero exit code",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: oneMinuteAgo,
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					InitContainerStatuses: []corev1.ContainerStatus{
						{
							RestartCount: 0,
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode: 1,
								},
							},
						},
					},
				},
			},
			now:      now,
			expected: false,
		},
		{
			name: "init container with restart count greater than 0",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: oneMinuteAgo,
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					InitContainerStatuses: []corev1.ContainerStatus{
						{
							RestartCount: 1,
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{},
							},
						},
					},
				},
			},
			now:      now,
			expected: false,
		},
		{
			name: "pod with no container statuses",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: oneMinuteAgo,
				},
				Status: corev1.PodStatus{
					Phase:             corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{},
				},
			},
			now:      now,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsHealthyPod(tt.pod, tt.now)
			assert.Equal(t, tt.expected, result)
		})
	}
}
