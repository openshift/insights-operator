package conditional

import (
	"fmt"

	"k8s.io/klog/v2"
)

func getAlertPodName(labels AlertLabels) (string, error) {
	name, ok := labels["pod"]
	if !ok {
		newErr := fmt.Errorf("alert is missing 'pod' label")
		klog.Warningln(newErr.Error())
		return "", newErr
	}
	return name, nil
}

func getAlertPodNamespace(labels AlertLabels) (string, error) {
	namespace, ok := labels["namespace"]
	if !ok {
		newErr := fmt.Errorf("alert is missing 'namespace' label")
		klog.Warningln(newErr.Error())
		return "", newErr
	}
	return namespace, nil
}
