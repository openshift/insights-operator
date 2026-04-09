package resources

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

// resourceType describes the type of Kubernetes resource for logging
type resourceType string

const (
	resourceTypeConfigMap resourceType = "ConfigMap"
	resourceTypeService   resourceType = "Service"
)

// loadYAMLResource unmarshals YAML into the provided object
func loadYAMLResource(data []byte, obj interface{}, resourceType resourceType) error {
	if err := yaml.Unmarshal(data, obj); err != nil {
		return fmt.Errorf("failed to unmarshal runtime extractor %s: %w", resourceType, err)
	}
	return nil
}

// deleteResource removes a Kubernetes resource by name
func deleteResource(deleteFn func() error, namespace, name string, resourceType resourceType) error {
	err := deleteFn()
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.Infof("Runtime extractor %s %s/%s already deleted", resourceType, namespace, name)
			return nil
		}
		return fmt.Errorf("failed to delete runtime extractor %s: %w", resourceType, err)
	}

	klog.Infof("Runtime extractor %s %s/%s deleted", resourceType, namespace, name)
	return nil
}

// resourceExists checks if a Kubernetes resource exists
func resourceExists(getFn func() (interface{}, error), namespace, name string, resourceType resourceType) bool {
	_, err := getFn()
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.Infof("[RuntimeExtractorController]: %s not found: %s, %v", resourceType, name, err)
			return false
		}
		klog.Errorf("Failed to get runtime extractor %s %s/%s: %v", resourceType, namespace, name, err)
		return false
	}
	return true
}
