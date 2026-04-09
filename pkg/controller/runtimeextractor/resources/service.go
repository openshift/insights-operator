//nolint:dupl // ConfigMap and Service files have similar structure but handle different resource types
package resources

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

const serviceName = "exporter"

//go:embed manifests/runtime-extractor-service.yaml
var runtimeExtractorServiceYAML []byte

// loadRuntimeExtractorService loads the embedded Service YAML and unmarshals it
func loadRuntimeExtractorService() (*corev1.Service, error) {
	svc := &corev1.Service{}
	if err := loadYAMLResource(runtimeExtractorServiceYAML, svc, resourceTypeService); err != nil {
		return nil, err
	}
	return svc, nil
}

// applyService creates or updates the runtime extractor Service
func (rm *ResourceManager) applyService(ctx context.Context) (*corev1.Service, error) {
	klog.Info("[RuntimeExtractorController]: applyService")

	required, err := loadRuntimeExtractorService()
	if err != nil {
		return nil, err
	}

	// ApplyService handles create/update logic with generation tracking
	svc, modified, err := resourceapply.ApplyService(ctx, rm.coreClient, rm.recorder, required)
	if err != nil {
		return nil, fmt.Errorf("failed to apply runtime extractor service: %w", err)
	}

	if modified {
		klog.Infof("Runtime extractor Service %s/%s was created or updated", svc.Namespace, svc.Name)
	}

	return svc, nil
}

// deleteService removes the runtime extractor Service
func (rm *ResourceManager) deleteService(ctx context.Context) error {
	return deleteResource(
		func() error {
			return rm.coreClient.Services(daemonSetNamespace).Delete(ctx, serviceName, metav1.DeleteOptions{})
		},
		daemonSetNamespace, serviceName, resourceTypeService)
}

// serviceExists checks if the runtime extractor Service exists
func (rm *ResourceManager) serviceExists(ctx context.Context) bool {
	return resourceExists(
		func() (interface{}, error) {
			return rm.coreClient.Services(daemonSetNamespace).Get(ctx, serviceName, metav1.GetOptions{})
		},
		daemonSetNamespace, serviceName, resourceTypeService)
}
