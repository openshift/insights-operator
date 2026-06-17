package runtimeextractor

import (
	"context"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

const (
	runtimeExtractorNamespace = "openshift-insights"
	runtimeExtractorName      = "insights-runtime-extractor"
)

// resourceInformer watches DaemonSet resources for external modifications
type resourceInformer struct {
	factory.Controller
	modifiedCh chan struct{}
}

// NewResourceInformer creates a new informer that watches runtime-extractor resources
// and notifies when they are modified by external actors (not by insights-operator)
//
//nolint:revive
func NewResourceInformer(
	eventRecorder events.Recorder,
	kubeInformers informers.SharedInformerFactory,
) (*resourceInformer, error) {
	ri := &resourceInformer{
		modifiedCh: make(chan struct{}, 10), // Buffered to prevent blocking
	}

	// Watch DaemonSet changes
	dsInformer := kubeInformers.Apps().V1().DaemonSets().Informer()
	_, err := dsInformer.AddEventHandler(ri.daemonSetEventHandler())
	if err != nil {
		return nil, err
	}

	// Create controller with all informers
	ctrl := factory.New().
		WithInformers(dsInformer).
		WithSync(ri.sync).
		ToController("RuntimeExtractorResourceInformer", eventRecorder)

	ri.Controller = ctrl
	return ri, nil
}

func (ri *resourceInformer) sync(_ context.Context, _ factory.SyncContext) error {
	// Sync is called after initial cache population
	// We don't need to do anything here as we're just watching for changes
	return nil
}

// daemonSetEventHandler handles DaemonSet modification events
func (ri *resourceInformer) daemonSetEventHandler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, newObj interface{}) {
			ri.handleDaemonSetUpdate(oldObj, newObj)
		},
		DeleteFunc: func(obj interface{}) {
			ri.handleResourceDeletion(obj, "DaemonSet")
		},
	}
}

// handleDaemonSetUpdate processes DaemonSet update events
func (ri *resourceInformer) handleDaemonSetUpdate(oldObj, newObj interface{}) {
	oldDS, ok := oldObj.(*appsv1.DaemonSet)
	if !ok {
		return
	}
	newDS, ok := newObj.(*appsv1.DaemonSet)
	if !ok {
		return
	}

	// Only care about our specific DaemonSet
	if newDS.Namespace != runtimeExtractorNamespace || newDS.Name != runtimeExtractorName {
		return
	}

	// Detect meaningful changes (ignore resourceVersion and generation changes from our own updates)
	if ri.isDaemonSetModified(oldDS, newDS) {
		klog.Infof("Runtime extractor DaemonSet %s/%s was modified externally, triggering reconciliation",
			newDS.Namespace, newDS.Name)
		ri.notifyModification()
	}
}

// handleResourceDeletion processes resource deletion events
func (ri *resourceInformer) handleResourceDeletion(obj interface{}, resourceType string) {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		klog.Errorf("Failed to get metadata from deleted runtime extractor %s: %v", resourceType, err)
		return
	}

	// Only care about our specific resources
	if metadata.GetNamespace() != runtimeExtractorNamespace {
		return
	}

	name := metadata.GetName()
	if name == runtimeExtractorName {
		klog.Infof("Runtime extractor %s %s/%s was deleted externally, triggering reconciliation",
			resourceType, metadata.GetNamespace(), name)
		ri.notifyModification()
	}
}

// isDaemonSetModified checks if the DaemonSet was meaningfully modified
// Uses generation to filter out status-only updates
func (ri *resourceInformer) isDaemonSetModified(oldObj, newObj *appsv1.DaemonSet) bool {
	// Generation only increments when spec changes (not status)
	// This filters out ~90% of update events (status updates, reconciliation loops)
	// resourceapply.ApplyDaemonSet will handle detailed comparison and decide if update is needed
	return oldObj.Generation != newObj.Generation
}

// notifyModification sends a notification to the modification channel (non-blocking)
func (ri *resourceInformer) notifyModification() {
	select {
	case ri.modifiedCh <- struct{}{}:
		// Notification sent
	default:
		// Channel is full, notification already pending - this is expected and safe to ignore
	}
}

// ResourceModified returns a channel that receives notifications when resources are modified
func (ri *resourceInformer) ResourceModified() <-chan struct{} {
	return ri.modifiedCh
}
