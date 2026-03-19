package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_IsContainerInCrashloop(t *testing.T) {
	tests := []struct {
		name     string
		status   *corev1.ContainerStatus
		expected bool
	}{
		{
			name: "container in crashloop with terminated state and non-zero exit code",
			status: &corev1.ContainerStatus{
				RestartCount: 3,
				LastTerminationState: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 1,
					},
				},
			},
			expected: true,
		},
		{
			name: "container in crashloop with waiting state",
			status: &corev1.ContainerStatus{
				RestartCount: 2,
				LastTerminationState: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason: "CrashLoopBackOff",
					},
				},
			},
			expected: true,
		},
		{
			name: "container with zero restart count",
			status: &corev1.ContainerStatus{
				RestartCount: 0,
				LastTerminationState: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 1,
					},
				},
			},
			expected: false,
		},
		{
			name: "container with restarts but exit code zero",
			status: &corev1.ContainerStatus{
				RestartCount: 1,
				LastTerminationState: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 0,
					},
				},
			},
			expected: false,
		},
		{
			name: "container with restarts but no termination state",
			status: &corev1.ContainerStatus{
				RestartCount:         1,
				LastTerminationState: corev1.ContainerState{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsContainerInCrashloop(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_HasContainerInCrashloop(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name: "pod with crashlooping regular container",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							RestartCount: 3,
							LastTerminationState: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode: 1,
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "pod with crashlooping init container",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					InitContainerStatuses: []corev1.ContainerStatus{
						{
							RestartCount: 2,
							LastTerminationState: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
									Reason: "CrashLoopBackOff",
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "pod with no crashlooping containers",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
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
			expected: false,
		},
		{
			name: "pod with no container statuses",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					InitContainerStatuses: []corev1.ContainerStatus{},
					ContainerStatuses:     []corev1.ContainerStatus{},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasContainerInCrashloop(tt.pod)
			assert.Equal(t, tt.expected, result)
		})
	}
}
