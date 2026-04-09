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

const configMapName = "kube-rbac-proxy"

//go:embed manifests/runtime-extractor-configmap.yaml
var runtimeExtractorConfigMapYAML []byte

// loadRuntimeExtractorConfigMap loads the embedded ConfigMap YAML and unmarshals it
func loadRuntimeExtractorConfigMap() (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	if err := loadYAMLResource(runtimeExtractorConfigMapYAML, cm, resourceTypeConfigMap); err != nil {
		return nil, err
	}
	return cm, nil
}

// applyConfigMap creates or updates the runtime extractor ConfigMap
func (rm *ResourceManager) applyConfigMap(ctx context.Context) (*corev1.ConfigMap, error) {
	klog.Info("[RuntimeExtractorController]: applyConfigMap")

	required, err := loadRuntimeExtractorConfigMap()
	if err != nil {
		return nil, err
	}

	// ApplyConfigMap handles create/update logic with generation tracking
	cm, modified, err := resourceapply.ApplyConfigMap(ctx, rm.coreClient, rm.recorder, required)
	if err != nil {
		return nil, fmt.Errorf("failed to apply runtime extractor configmap: %w", err)
	}

	if modified {
		klog.Infof("Runtime extractor ConfigMap %s/%s was created or updated", cm.Namespace, cm.Name)
	}

	return cm, nil
}

// deleteConfigMap removes the runtime extractor ConfigMap
func (rm *ResourceManager) deleteConfigMap(ctx context.Context) error {
	return deleteResource(
		func() error {
			return rm.coreClient.ConfigMaps(daemonSetNamespace).Delete(ctx, configMapName, metav1.DeleteOptions{})
		},
		daemonSetNamespace, configMapName, resourceTypeConfigMap)
}

// configMapExists checks if the runtime extractor ConfigMap exists
func (rm *ResourceManager) configMapExists(ctx context.Context) bool {
	return resourceExists(
		func() (interface{}, error) {
			return rm.coreClient.ConfigMaps(daemonSetNamespace).Get(ctx, configMapName, metav1.GetOptions{})
		},
		daemonSetNamespace, configMapName, resourceTypeConfigMap)
}
