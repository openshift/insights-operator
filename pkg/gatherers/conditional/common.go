package conditional

import (
	"fmt"
)

func getAlertPodName(labels AlertLabels) (string, error) {
	name, ok := labels["pod"]
	if !ok {
		newErr := fmt.Errorf("alert is missing 'pod' label")
		return "", newErr
	}
	return name, nil
}

func getAlertPodNamespace(labels AlertLabels) (string, error) {
	namespace, ok := labels["namespace"]
	if !ok {
		newErr := fmt.Errorf("alert is missing 'namespace' label")
		return "", newErr
	}
	return namespace, nil
}

func getAlertPodContainer(labels AlertLabels) (string, error) {
	container, ok := labels["container"]
	if !ok && len(container) > 0 {
		newErr := fmt.Errorf("alert is missing 'container' label")
		return "", newErr
	}
	return container, nil
}
