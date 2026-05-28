package resources

import (
	"context"
	"fmt"

	"github.com/openshift/library-go/pkg/operator/events"
	appsclientv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
)

// ResourceManager manages the lifecycle of runtime extractor resources (DaemonSet)
type ResourceManager struct {
	daemonSetGetterClient appsclientv1.DaemonSetsGetter
	recorder              events.Recorder
}

// NewResourceManager creates a new ResourceManager for managing runtime extractor resources
func NewResourceManager(
	daemonSetGetterClient appsclientv1.DaemonSetsGetter,
	recorder events.Recorder,
) *ResourceManager {
	return &ResourceManager{
		daemonSetGetterClient: daemonSetGetterClient,
		recorder:              recorder,
	}
}

// ApplyRuntimeExtractorResources creates or updates the runtime extractor DaemonSet
// This should be called when the runtime extractor should be deployed or updated
func (rm *ResourceManager) ApplyRuntimeExtractorResources(
	ctx context.Context,
) error {
	if _, err := rm.applyDaemonSet(ctx); err != nil {
		return fmt.Errorf("failed to apply daemonset: %v", err)
	}

	return nil
}

// DeleteRuntimeExtractorResources removes the runtime extractor DaemonSet
// This should be called when the runtime extractor should be disabled
func (rm *ResourceManager) DeleteRuntimeExtractorResources(
	ctx context.Context,
) error {
	if err := rm.deleteDaemonSet(ctx); err != nil {
		return fmt.Errorf("failed to delete daemonset: %v", err)
	}

	return nil
}

// ResourcesExists checks if the runtime extractor DaemonSet is already created.
func (rm *ResourceManager) ResourcesExists(ctx context.Context) bool {
	return rm.daemonSetExists(ctx)
}
