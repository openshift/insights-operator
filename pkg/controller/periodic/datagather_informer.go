package periodic

import (
	"context"
	"slices"
	"strings"

	insightsv1 "github.com/openshift/api/insights/v1"
	insightsInformers "github.com/openshift/client-go/insights/informers/externalversions"
	insightsListers "github.com/openshift/client-go/insights/listers/insights/v1"
	"github.com/openshift/insights-operator/pkg/controller/status"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

const (
	periodicGatheringPrefix = "periodic-gathering-"
)

// DataGatherInformer is an interface providing information
// about newly created DataGather resources
type DataGatherInformer interface {
	// Controller provides the base controller functionality from library-go
	factory.Controller
	// DataGatherCreated returns a receive-only channel that sends the name of newly created
	// DataGather resources based on which the on-demand gathering is triggered
	DataGatherCreated() <-chan string
	// Lister returns a DataGatherLister that provides cached access to all DataGather resources
	// without making API requests to the Kubernetes API server
	Lister() insightsListers.DataGatherLister
	// DataGatherStatusChanged returns a receive-only channel that signals when a DataGather
	// resource's status changes to a finished state (GatheringFailed or GatheringSucceeded).
	// This is used to check if data gathering has completed and trigger reconciliation of pending gatherings.
	DataGatherStatusChanged() <-chan struct{}
}

// dataGatherController is type implementing DataGatherInformer
type dataGatherController struct {
	factory.Controller
	ch            chan string
	lister        insightsListers.DataGatherLister
	statusChanged chan struct{}
}

// NewDataGatherInformer creates a new instance of the DataGatherInformer interface
func NewDataGatherInformer(eventRecorder events.Recorder, insightsInf insightsInformers.SharedInformerFactory) (DataGatherInformer, error) {
	inf := insightsInf.Insights().V1().DataGathers().Informer()
	lister := insightsInf.Insights().V1().DataGathers().Lister()

	dgCtrl := &dataGatherController{
		ch:            make(chan string),
		statusChanged: make(chan struct{}, 10), // buffered
		lister:        lister,
	}
	_, err := inf.AddEventHandler(dgCtrl.eventHandler())
	if err != nil {
		return nil, err
	}

	ctrl := factory.New().WithInformers(inf).
		WithSync(dgCtrl.sync).
		ToController("DataGatherInformer", eventRecorder)

	dgCtrl.Controller = ctrl
	return dgCtrl, nil
}

func (d *dataGatherController) sync(_ context.Context, _ factory.SyncContext) error {
	return nil
}

// eventHandler returns a new ResourceEventHandler that handles the DataGather resources
// addition events. Resources with the prefix "periodic-gathering-" are filtered out to avoid conflicts
// with periodic data gathering.
func (d *dataGatherController) eventHandler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			dgMetadata, err := meta.Accessor(obj)
			if err != nil {
				klog.Errorf("Can't read metadata of newly added DataGather resource: %v", err)
				return
			}
			// filter out dataGathers created for periodic gathering
			if strings.HasPrefix(dgMetadata.GetName(), periodicGatheringPrefix) {
				return
			}
			d.ch <- dgMetadata.GetName()
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldDG, ok := oldObj.(*insightsv1.DataGather)
			if !ok {
				klog.Errorf("Expected DataGather, got %T", oldObj)
				return
			}

			newDG, ok := newObj.(*insightsv1.DataGather)
			if !ok {
				klog.Errorf("Expected DataGather, got %T", newObj)
				return
			}

			// filter out dataGathers created for periodic gathering
			if strings.HasPrefix(newDG.GetName(), periodicGatheringPrefix) {
				return
			}

			newCondition := status.GetConditionByType(newDG, status.Progressing)
			finishedReasons := []string{status.GatheringFailedReason, status.GatheringSucceededReason}
			// Continue only if the new condition is one of the finished conditions
			if newCondition == nil || !slices.Contains(finishedReasons, newCondition.Reason) {
				return
			}

			oldCondition := status.GetConditionByType(oldDG, status.Progressing)
			if oldCondition == nil {
				return
			}

			// Send signal only if the old condition is not equal to the new condition, which means
			// the state changed from running to some of the finished conditions.
			if oldCondition.Status != newCondition.Status ||
				oldCondition.Reason != newCondition.Reason ||
				!oldCondition.LastTransitionTime.Equal(&newCondition.LastTransitionTime) {
				klog.Infof("DataGather %s status changed, signaling reconciliation", newDG.Name)

				select {
				case d.statusChanged <- struct{}{}:
				default:
					// Channel full, signal already pending
				}
			}
		},
	}
}

// DataGatherCreated returns a channel providing the name of
// newly created DataGather resource
func (d *dataGatherController) DataGatherCreated() <-chan string {
	return d.ch
}

// Lister returns a DataGatherLister that can be used to query
// the informer's cache without making API requests
func (d *dataGatherController) Lister() insightsListers.DataGatherLister {
	return d.lister
}

// DataGatherStatusChanged returns a channel providing the name of
// updated DataGather resource
func (d *dataGatherController) DataGatherStatusChanged() <-chan struct{} {
	return d.statusChanged
}
