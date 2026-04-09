package resources

import (
	"context"
	"fmt"

	"github.com/openshift/library-go/pkg/operator/events"
	appsclientv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"
)

// ResourceManager manages the lifecycle of runtime extractor resources (ConfigMap, Service, DaemonSet)
type ResourceManager struct {
	daemonSetGetterClient appsclientv1.DaemonSetsGetter
	coreClient            corev1client.CoreV1Interface
	recorder              events.Recorder
}

// NewResourceManager creates a new ResourceManager for managing runtime extractor resources
func NewResourceManager(
	daemonSetGetterClient appsclientv1.DaemonSetsGetter,
	coreClient corev1client.CoreV1Interface,
	recorder events.Recorder,
) *ResourceManager {
	return &ResourceManager{
		daemonSetGetterClient: daemonSetGetterClient,
		coreClient:            coreClient,
		recorder:              recorder,
	}
}

// ApplyRuntimeExtractorResources creates or updates all runtime extractor resources (ConfigMap, Service, DaemonSet)
// This should be called when the runtime extractor should be deployed or updated
func (rm *ResourceManager) ApplyRuntimeExtractorResources(
	ctx context.Context,
) error {
	// Apply resources in dependency order: ConfigMap -> Service -> DaemonSet
	// ConfigMap is needed by the DaemonSet pods, Service is needed for TLS cert generation

	klog.Info("[RuntimeExtractorController]: ApplyRuntimeExtractorResources")

	if _, err := rm.applyConfigMap(ctx); err != nil {
		return fmt.Errorf("failed to apply configmap: %v", err)
	}

	if _, err := rm.applyService(ctx); err != nil {
		return fmt.Errorf("failed to apply service: %v", err)
	}

	if _, err := rm.applyDaemonSet(ctx); err != nil {
		return fmt.Errorf("failed to apply daemonset: %v", err)
	}

	return nil
}

// DeleteRuntimeExtractorResources removes all runtime extractor resources (DaemonSet, Service, ConfigMap)
// This should be called when the runtime extractor should be disabled
func (rm *ResourceManager) DeleteRuntimeExtractorResources(
	ctx context.Context,
) error {
	// Delete resources in reverse dependency order: DaemonSet -> Service -> ConfigMap
	var errs []error

	if err := rm.deleteDaemonSet(ctx); err != nil {
		errs = append(errs, fmt.Errorf("failed to delete daemonset: %w", err))
	}

	if err := rm.deleteService(ctx); err != nil {
		errs = append(errs, fmt.Errorf("failed to delete service: %w", err))
	}

	if err := rm.deleteConfigMap(ctx); err != nil {
		errs = append(errs, fmt.Errorf("failed to delete configmap: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors deleting runtime extractor resources: %v", errs)
	}

	return nil
}

// ResourcesCreated checks if runtime extractor resources (DaemonSet, Service, ConfigMap)
// are already created.
func (rm *ResourceManager) ResourcesExists(ctx context.Context) bool {
	cmExists := rm.configMapExists(ctx)
	svcExists := rm.serviceExists(ctx)
	dsExists := rm.daemonSetExists(ctx)

	exists := cmExists || svcExists || dsExists
	if cmExists != svcExists || svcExists != dsExists {
		klog.Errorf("[RuntimeExtractorController]: Inconsistent state detected - CM:%v Svc:%v DS:%v",
			cmExists, svcExists, dsExists)
	}

	return exists
}
