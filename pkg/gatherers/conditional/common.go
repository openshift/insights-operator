package conditional

import (
	"fmt"
)

var (
	ErrAlertPodNameMissing      = fmt.Errorf("alert is missing 'pod' label")
	ErrAlertPodNamespaceMissing = fmt.Errorf("alert is missing 'namespace' label")
	ErrAlertPodContainerMissing = fmt.Errorf("alert is missing 'container' label")
)

func getAlertPodName(labels AlertLabels) (string, error) {
	name, ok := labels["pod"]
	if !ok {
		return "", ErrAlertPodNameMissing
	}
	return name, nil
}

func getAlertPodNamespace(labels AlertLabels) (string, error) {
	namespace, ok := labels["namespace"]
	if !ok {
		return "", ErrAlertPodNamespaceMissing
	}
	return namespace, nil
}

func getAlertPodContainer(labels AlertLabels) (string, error) {
	container, ok := labels["container"]
	if !ok || len(container) == 0 {
		return "", ErrAlertPodContainerMissing
	}
	return container, nil
}
