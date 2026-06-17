package runtimeextractor

import (
	"context"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/controller/runtimeextractor/resources"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// ConfigNotifier provides access to Insights configuration and notifications about configuration changes
type ConfigNotifier interface {
	// ConfigChanged returns a channel that receives notifications when configuration changes
	// and a cleanup function to close the notification channel
	ConfigChanged() (<-chan struct{}, func())
	// Config returns the current Insights configuration
	Config() *config.InsightsConfiguration
}

// ResourceManager manages the lifecycle of runtime extractor Kubernetes resources
type ResourceManager interface {
	// ApplyRuntimeExtractorResources creates or updates all runtime extractor resources
	ApplyRuntimeExtractorResources(ctx context.Context) error
	// DeleteRuntimeExtractorResources removes all runtime extractor resources
	DeleteRuntimeExtractorResources(ctx context.Context) error
	// ResourcesExists checks if runtime extractor resources are deployed
	ResourcesExists(ctx context.Context) bool
}

// ResourceInformer provides notifications when runtime-extractor resources are modified
// externally (not by insights-operator). This enables drift detection and reconciliation.
type ResourceInformer interface {
	factory.Controller
	// ResourceModified returns a channel that receives notifications when resources are modified
	ResourceModified() <-chan struct{}
}

// runtimeExtractorController manages the lifecycle of runtime extractor resources in the cluster.
// It watches for configuration changes and cluster version updates, creating, updating, or deleting
// the runtime extractor DaemonSet and associated resources as needed.
//
// The controller responds to three primary events:
//   - Configuration changes: Creates or deletes resources based on DisableRuntimeExtractor flag
//   - Version updates: Updates DaemonSet container images to match the current cluster version
//   - Resource modifications: Detects and corrects external changes to runtime-extractor resources
type runtimeExtractorController struct {
	// config provides access to Insights configuration and notifications about configuration changes
	config ConfigNotifier
	// updateCh receives notifications when the cluster version changes, triggering DaemonSet image updates
	updateCh chan struct{}
	// ResourceInformer watches for external modifications to runtime-extractor resources
	ResourceInformer ResourceInformer
	// resourceManager handles creation, update, and deletion of runtime extractor Kubernetes resources
	resourceManager ResourceManager
}

// NewRuntimeExtractorController is a constructor for runtimeExtractorController
// that is in charge of runtime-extractor deployment lifecycle
//
//nolint:revive
func NewRuntimeExtractorController(
	configNotifier ConfigNotifier,
	updateCh chan struct{},
	kubeClient *kubernetes.Clientset,
	recorder events.Recorder,
	resourceInformer ResourceInformer,
) *runtimeExtractorController {
	rm := resources.NewResourceManager(
		kubeClient.AppsV1(),
		recorder,
	)

	return &runtimeExtractorController{
		config:           configNotifier,
		updateCh:         updateCh,
		resourceManager:  rm,
		ResourceInformer: resourceInformer,
	}
}

// Run starts the runtime extractor controller and handles configuration changes and version updates.
// It performs initial deployment based on configuration, then watches for:
//   - Configuration changes (create/delete resources based on DisableRuntimeExtractor flag)
//   - Cluster version updates (update DaemonSet images to match new cluster version)
//   - Resource modifications (detect and correct external changes to runtime-extractor resources)
//
// The controller runs until the context is canceled.
func (re *runtimeExtractorController) Run(ctx context.Context) {
	klog.Info("Starting runtime extractor controller")

	// Initial deploy of DaemonSet
	re.handleConfigChange(ctx)

	configChan, configClose := re.config.ConfigChanged()
	defer configClose()

	// Get resource modification notifications from informer
	resourceModifiedChan := re.ResourceInformer.ResourceModified()

	// Watch for configuration changes, version updates, and external resource modifications
	for {
		select {
		case <-configChan:
			klog.Info("Runtime extractor configuration changed")
			re.handleConfigChange(ctx)
		case <-re.updateCh:
			klog.Info("Runtime extractor cluster version updated")
			re.handleVersionUpdate(ctx)
		case <-resourceModifiedChan:
			klog.Info("Runtime extractor resources modified externally, reconciling")
			re.handleResourceDrift(ctx)
		case <-ctx.Done():
			klog.Info("Runtime extractor controller stopped")
			return
		}
	}
}

// handleConfigChange responds to configuration changes by creating or deleting runtime extractor resources
// based on the DisableRuntimeExtractor configuration flag.
func (re *runtimeExtractorController) handleConfigChange(ctx context.Context) {
	cfg := re.config.Config()

	if cfg.DataReporting.DisableRuntimeExtractor {
		klog.Info("Runtime extractor is disabled, deleting resources")
		re.deleteDeployment(ctx)
	} else {
		klog.Info("Runtime extractor is enabled, creating resources")
		re.createDeployment(ctx)
	}
}

// handleVersionUpdate responds to cluster version changes by updating the runtime extractor DaemonSet
// to use container images matching the new cluster version. Skips update if runtime extractor is disabled.
func (re *runtimeExtractorController) handleVersionUpdate(ctx context.Context) {
	cfg := re.config.Config()

	if cfg.DataReporting.DisableRuntimeExtractor {
		klog.Info("Runtime extractor is disabled, skipping version update")
		return
	}

	re.updateDeployment(ctx)
}

func (re *runtimeExtractorController) isCreated(ctx context.Context) bool {
	return re.resourceManager.ResourcesExists(ctx)
}

func (re *runtimeExtractorController) createDeployment(ctx context.Context) {
	klog.Info("Creating runtime extractor resources")

	if err := re.resourceManager.ApplyRuntimeExtractorResources(ctx); err != nil {
		klog.Errorf("Failed to apply runtime extractor resources: %v", err)
	}
}

func (re *runtimeExtractorController) deleteDeployment(ctx context.Context) {
	klog.Info("Deleting runtime extractor resources")

	if !re.isCreated(ctx) {
		klog.Info("Runtime extractor resources do not exist, nothing to delete")
		return
	}

	if err := re.resourceManager.DeleteRuntimeExtractorResources(ctx); err != nil {
		klog.Errorf("Failed to delete runtime extractor resources: %v", err)
	}
}

func (re *runtimeExtractorController) updateDeployment(ctx context.Context) {
	klog.Info("Updating runtime extractor resources")

	// Avoid creating it when the cluster version is updated
	if !re.isCreated(ctx) {
		klog.Info("Runtime extractor resources not found, skipping update")
		return
	}

	if err := re.resourceManager.ApplyRuntimeExtractorResources(ctx); err != nil {
		klog.Errorf("Failed to apply runtime extractor resources: %v", err)
	}
}

// handleResourceDrift responds to external modifications of runtime-extractor resources
// by reapplying the desired state. This ensures that any manual changes or deletions
// are automatically corrected to maintain the insights-operator's desired configuration.
func (re *runtimeExtractorController) handleResourceDrift(ctx context.Context) {
	cfg := re.config.Config()

	// Only reconcile if runtime extractor should be enabled
	if cfg.DataReporting.DisableRuntimeExtractor {
		klog.Info("Runtime extractor is disabled, ensuring resources are absent")
		re.deleteDeployment(ctx)
		return
	}

	// Reapply resources to correct any drift
	if err := re.resourceManager.ApplyRuntimeExtractorResources(ctx); err != nil {
		klog.Errorf("Failed to correct runtime extractor resource drift: %v", err)
	} else {
		klog.Info("Successfully reconciled runtime extractor resources")
	}
}
