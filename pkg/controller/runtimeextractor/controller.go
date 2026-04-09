package runtimeextractor

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/controller/runtimeextractor/resources"
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

// runtimeExtractorController manages the lifecycle of runtime extractor resources in the cluster.
// It watches for configuration changes and cluster version updates, creating, updating, or deleting
// the runtime extractor DaemonSet and associated resources as needed.
//
// The controller responds to two primary events:
//   - Configuration changes: Creates or deletes resources based on DisableRuntimeExtractor flag
//   - Version updates: Updates DaemonSet container images to match the current cluster version
type runtimeExtractorController struct {
	// config provides access to Insights configuration and notifications about configuration changes
	config ConfigNotifier
	// updateCh receives notifications when the cluster version changes, triggering DaemonSet image updates
	updateCh chan struct{}
	// resourceManager handles creation, update, and deletion of runtime extractor Kubernetes resources
	resourceManager ResourceManager
}

// NewRuntimeExtractorController is a constructor for runtimeExtractorController
// that is in charge of runtime-extractor deployment lifecycle
func NewRuntimeExtractorController(
	configNotifier ConfigNotifier,
	updateCh chan struct{},
	kubeClient *kubernetes.Clientset,
	recorder events.Recorder,
) *runtimeExtractorController {
	rm := resources.NewResourceManager(
		// TODO: maybe it could be done in a better way
		kubeClient.AppsV1(),
		kubeClient.CoreV1(),
		recorder,
	)

	return &runtimeExtractorController{
		config:          configNotifier,
		updateCh:        updateCh,
		resourceManager: rm,
	}
}

// Run starts the runtime extractor controller and handles configuration changes and version updates.
// It performs initial deployment based on configuration, then watches for:
//   - Configuration changes (create/delete resources based on DisableRuntimeExtractor flag)
//   - Cluster version updates (update DaemonSet images to match new cluster version)
//
// The controller runs until the context is cancelled.
func (re *runtimeExtractorController) Run(ctx context.Context) {
	klog.Info("[RuntimeExtractorController]: Run")

	// !!! TODO: if the pod restarts and the env is changed it will not
	// update daemonset images correctly
	re.handleConfigChange(ctx)

	configChan, configClose := re.config.ConfigChanged()
	defer configClose()

	// Check ConfigMap if the DisableRuntimeExtractor is set
	// Based on that Create/Delete the RuntimeExtractor deployment
	// Also watch for Updates if the deployment needs some changes
	for {
		select {
		case <-configChan:
			klog.Info("[RuntimeExtractorController]: Configuration Changed")
			// Check if disableRuntimeExtractor was changed
			re.handleConfigChange(ctx)
		case <-re.updateCh:
			klog.Infof("[RuntimeExtractorController]: Version bumped")
			re.handleVersionUpdate(ctx)
		case <-ctx.Done():
			klog.Info("[RuntimeExtractorController]: Context Done")
			return
		}
	}
}

// handleConfigChange responds to configuration changes by creating or deleting runtime extractor resources
// based on the DisableRuntimeExtractor configuration flag.
// TODO: do we need an mutex here?
func (re *runtimeExtractorController) handleConfigChange(ctx context.Context) {
	klog.Info("[RuntimeExtractorController]: handleConfigChange")
	cfg := re.config.Config()

	var err error
	if cfg.DataReporting.DisableRuntimeExtractor {
		klog.Info("RuntimeExtractor is disabled")
		err = re.deleteDeployment(ctx)
	} else {
		klog.Info("RuntimeExtractor is enabled")
		err = re.createDeployment(ctx)
	}

	if err != nil {
		klog.Errorf("[RuntimeExtractorController]: Failed to handle config change: %v", err)
		// TODO: Consider adding retry mechanism or status reporting
	}
}

// handleVersionUpdate responds to cluster version changes by updating the runtime extractor DaemonSet
// to use container images matching the new cluster version. Skips update if runtime extractor is disabled.
// TODO: do we need an mutex here?
func (re *runtimeExtractorController) handleVersionUpdate(ctx context.Context) {
	klog.Info("[RuntimeExtractorController]: Update Deployment")
	cfg := re.config.Config()

	if cfg.DataReporting.DisableRuntimeExtractor {
		return
	}
	// TODO: version
	// TODO: handle error - retry or something?
	re.updateDeployment(ctx)
}

func (re *runtimeExtractorController) isCreated(ctx context.Context) bool {
	return re.resourceManager.ResourcesExists(ctx)
}

func (re *runtimeExtractorController) createDeployment(ctx context.Context) error {
	klog.Info("[RuntimeExtractorController]: Create Deployment")

	// TODO: we need to make sure it uses the latest images so we should run apply
	// if re.isCreated(ctx) {
	// 	klog.Info("[RuntimeExtractorController]: Resources Already Exists")
	// 	return nil
	// }

	if err := re.resourceManager.ApplyRuntimeExtractorResources(ctx); err != nil {
		klog.Errorf("Failed to ApplyRuntimeExtractorResources: %v", err)
		return err
	}

	return nil
}

func (re *runtimeExtractorController) deleteDeployment(ctx context.Context) error {
	klog.Info("[RuntimeExtractorController]: Delete Deployment")

	if !re.isCreated(ctx) {
		klog.Info("[RuntimeExtractorController]: Resources Not Exists")
		return nil
	}

	if err := re.resourceManager.DeleteRuntimeExtractorResources(ctx); err != nil {
		klog.Errorf("Failed to DeleteRuntimeExtractorResources: %v", err)
		return err
	}

	return nil
}

func (re *runtimeExtractorController) updateDeployment(ctx context.Context) error {
	klog.Info("[RuntimeExtractorController]: Update Deployment")

	// Avoid creating it when the cluster version is updated
	if !re.isCreated(ctx) {
		klog.Errorf("[RuntimeExtractorController]: Resources Not Found")
		return fmt.Errorf("can not update not created resources")
	}

	if err := re.resourceManager.ApplyRuntimeExtractorResources(ctx); err != nil {
		klog.Errorf("Failed to ApplyRuntimeExtractorResources: %v", err)
		return err
	}

	return nil
}
