package check

import corev1 "k8s.io/api/core/v1"

func IsContainerInCrashloop(status *corev1.ContainerStatus) bool {
	return status.RestartCount > 0 &&
		((status.LastTerminationState.Terminated != nil &&
			status.LastTerminationState.Terminated.ExitCode != 0) ||
			status.LastTerminationState.Waiting != nil)
}

func HasContainerInCrashloop(pod *corev1.Pod) bool {
	for idx := range pod.Status.InitContainerStatuses {
		if IsContainerInCrashloop(&pod.Status.InitContainerStatuses[idx]) {
			return true
		}
	}
	for idx := range pod.Status.ContainerStatuses {
		if IsContainerInCrashloop(&pod.Status.ContainerStatuses[idx]) {
			return true
		}
	}
	return false
}
