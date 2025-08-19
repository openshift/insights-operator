package periodic

import (
	"context"
	"strings"

	insightsInformers "github.com/openshift/client-go/insights/informers/externalversions"
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
// about newly create DataGather resources
type DataGatherInformer interface {
	factory.Controller
	DataGatherCreated() <-chan string
}

// dataGatherController is type implementing DataGatherInformer
type dataGatherController struct {
	factory.Controller
	ch chan string
}

// NewDataGatherInformer creates a new instance of the DataGatherInformer interface
func NewDataGatherInformer(eventRecorder events.Recorder, insightsInf insightsInformers.SharedInformerFactory) (DataGatherInformer, error) {
	inf := insightsInf.Insights().V1().DataGathers().Informer()

	dgCtrl := &dataGatherController{
		ch: make(chan string),
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
	}
}

// DataGatherCreated returns a channel providing the name of
// newly created DataGather resource
func (d *dataGatherController) DataGatherCreated() <-chan string {
	return d.ch
}
