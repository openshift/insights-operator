package status

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/controllerstatus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	// How many upload failures in a row we tolerate before starting reporting
	// as InsightsUploadDegraded
	uploadFailuresCountThreshold = 5
	// GatherFailuresCountThreshold defines how many gatherings can fail in a row before we report Degraded
	GatherFailuresCountThreshold = 5
	// OCMAPIFailureCountThreshold defines how many unsuccessful responses from the OCM API in a row is tolerated
	// before the operator is marked as Degraded
	OCMAPIFailureCountThreshold = 5
)

type Reported struct {
	LastReportTime metav1.Time `json:"lastReportTime"`
}

type Configurator interface {
	Config() *config.Controller
}

// Controller is the type responsible for managing the statusMessage of the operator according to the statusMessage of the sources.
// Sources come from different major parts of the codebase, for the purpose of communicating their statusMessage with the controller.
type Controller struct {
	name      string
	namespace string

	client configv1client.ConfigV1Interface

	statusCh     chan struct{}
	configurator Configurator

	sources  []controllerstatus.Interface
	reported Reported
	start    time.Time

	ctrlStatus *controllerStatus

	lock sync.Mutex
}

// NewController creates a statusMessage controller, responsible for monitoring the operators statusMessage and updating its cluster statusMessage accordingly.
func NewController(client configv1client.ConfigV1Interface, configurator Configurator, namespace string) *Controller {
	c := &Controller{
		name:         "insights",
		statusCh:     make(chan struct{}, 1),
		configurator: configurator,
		client:       client,
		namespace:    namespace,
		ctrlStatus:   newControllerStatus(),
	}
	return c
}

func (c *Controller) triggerStatusUpdate() {
	select {
	case c.statusCh <- struct{}{}:
	default:
	}
}

func (c *Controller) controllerStartTime() time.Time {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.start.IsZero() {
		c.start = time.Now()
	}
	return c.start
}

// LastReportedTime provides the last reported time in a thread-safe way.
func (c *Controller) LastReportedTime() time.Time {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.reported.LastReportTime.Time
}

// SetLastReportedTime sets the last reported time in a thread-safe way.
func (c *Controller) SetLastReportedTime(at time.Time) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.reported.LastReportTime.IsZero() {
		klog.V(2).Infof("Initializing last reported time to %s", at.UTC().Format(time.RFC3339))
	}
	c.reported.LastReportTime.Time = at
	c.triggerStatusUpdate()
}

// AddSources adds sources in a thread-safe way.
// A source is used to monitor parts of the operator.
func (c *Controller) AddSources(sources ...controllerstatus.Interface) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.sources = append(c.sources, sources...)
}

// Sources provides the sources in a thread-safe way.
// A source is used to monitor parts of the operator.
func (c *Controller) Sources() []controllerstatus.Interface {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.sources
}

func (c *Controller) merge(clusterOperator *configv1.ClusterOperator) *configv1.ClusterOperator {
	// prime the object if it does not exist
	if clusterOperator == nil {
		clusterOperator = newClusterOperator(c.name, nil)
	}

	// make sure to start a clean status controller
	c.ctrlStatus.reset()

	// calculate the current controller state
	allReady, lastTransition := c.currentControllerStatus()

	clusterOperator = clusterOperator.DeepCopy()
	now := time.Now()
	if len(c.namespace) > 0 {
		clusterOperator.Status.RelatedObjects = relatedObjects(c.namespace)
	}

	isInitializing := !allReady && now.Sub(c.controllerStartTime()) < 3*time.Minute

	// cluster operator conditions
	cs := newConditions(&clusterOperator.Status, metav1.Time{Time: now})
	updateControllerConditions(cs, c.ctrlStatus, isInitializing, lastTransition)

	// once the operator is running it is always considered available
	cs.setCondition(configv1.OperatorAvailable, configv1.ConditionTrue, "AsExpected", "", metav1.Now())

	updateControllerConditionsByStatus(cs, c.ctrlStatus, isInitializing, lastTransition)

	// all status conditions from conditions to cluster operator
	clusterOperator.Status.Conditions = cs.entries()

	if release := os.Getenv("RELEASE_VERSION"); len(release) > 0 {
		clusterOperator.Status.Versions = []configv1.OperandVersion{
			{Name: "operator", Version: release},
		}
	}

	reported := Reported{LastReportTime: metav1.Time{Time: c.LastReportedTime()}}
	if data, err := json.Marshal(reported); err != nil {
		klog.Errorf("Unable to marshal statusMessage extension: %v", err)
	} else {
		clusterOperator.Status.Extension.Raw = data
	}
	return clusterOperator
}

// calculate the current controller status based on its given sources
func (c *Controller) currentControllerStatus() (allReady bool, lastTransition time.Time) {
	var errorReason string
	var errs []string

	allReady = true

	for i, source := range c.Sources() {
		summary, ready := source.CurrentStatus()
		if !ready {
			klog.V(4).Infof("Source %d %T is not ready", i, source)
			allReady = false
			continue
		}
		if summary.Healthy {
			continue
		}
		if len(summary.Message) == 0 {
			klog.Errorf("Programmer error: statusMessage source %d %T reported an empty message: %#v", i, source, summary)
			continue
		}

		degradingFailure := false

		if summary.Operation == controllerstatus.Uploading {
			if summary.Count < uploadFailuresCountThreshold {
				klog.V(4).Infof("Number of last upload failures %d lower than threshold %d. Not marking as degraded.",
					summary.Count, uploadFailuresCountThreshold)
			} else {
				degradingFailure = true
				klog.V(4).Infof("Number of last upload failures %d exceeded the threshold %d. Marking as degraded.",
					summary.Count, uploadFailuresCountThreshold)
			}
			c.ctrlStatus.setStatus(UploadStatus, summary.Reason, summary.Message)
		} else if summary.Operation == controllerstatus.DownloadingReport {
			klog.V(4).Info("Failed to download Insights report")
			c.ctrlStatus.setStatus(DownloadStatus, summary.Reason, summary.Message)
		} else if summary.Operation == controllerstatus.PullingSCACerts {
			klog.V(4).Infof("Failed to download the SCA certs within the threshold %d with exponential backoff. Marking as degraded.",
				OCMAPIFailureCountThreshold)
			degradingFailure = true
		}

		if degradingFailure {
			errorReason = summary.Reason
			errs = append(errs, summary.Message)
		}

		if lastTransition.Before(summary.LastTransitionTime) {
			lastTransition = summary.LastTransitionTime
		}
	}

	// handling errors
	errorReason, errorMessage := handleControllerStatusError(errs, errorReason)
	if errorReason != "" || errorMessage != "" {
		c.ctrlStatus.setStatus(ErrorStatus, errorReason, errorMessage)
	}

	// disabled state only when it's disabled by config. It means that gathering will not happen
	if !c.configurator.Config().Report {
		c.ctrlStatus.setStatus(DisabledStatus, "Disabled", "Health reporting is disabled")
	}

	return allReady, lastTransition
}

// Start starts the periodic checking of sources.
func (c *Controller) Start(ctx context.Context) error {
	if err := c.updateStatus(ctx, true); err != nil {
		return err
	}
	limiter := rate.NewLimiter(rate.Every(30*time.Second), 2)
	go wait.Until(func() {
		timer := time.NewTicker(2 * time.Minute)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
			case <-timer.C:
				err := limiter.Wait(ctx)
				if err != nil {
					klog.Errorf("Limiter error by timer: %v", err)
				}
			case <-c.statusCh:
				err := limiter.Wait(ctx)
				if err != nil {
					klog.Errorf("Limiter error by statusMessage: %v", err)
				}
			}
			if err := c.updateStatus(ctx, false); err != nil {
				klog.Errorf("Unable to write cluster operator statusMessage: %v", err)
			}
		}
	}, time.Second, ctx.Done())
	return nil
}

func (c *Controller) updateStatus(ctx context.Context, initial bool) error {
	existing, err := c.client.ClusterOperators().Get(ctx, c.name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		existing = nil
	}
	if initial {
		if existing != nil {
			var reported Reported
			if len(existing.Status.Extension.Raw) > 0 {
				if err := json.Unmarshal(existing.Status.Extension.Raw, &reported); err != nil { //nolint: govet
					klog.Errorf("The initial operator extension statusMessage is invalid: %v", err)
				}
			}
			c.SetLastReportedTime(reported.LastReportTime.Time.UTC())
			cs := newConditions(&existing.Status, metav1.Now())
			if con := cs.findCondition(configv1.OperatorDegraded); con == nil ||
				con != nil && con.Status == configv1.ConditionFalse {
				klog.Info("The initial operator extension statusMessage is healthy")
			}
		}
	}

	updated := c.merge(existing)
	if existing == nil {
		created, err := c.client.ClusterOperators().Create(ctx, updated, metav1.CreateOptions{}) //nolint: govet
		if err != nil {
			return err
		}
		updated.ObjectMeta = created.ObjectMeta
		updated.Spec = created.Spec
	} else if reflect.DeepEqual(updated.Status, existing.Status) {
		klog.V(4).Infof("No statusMessage update necessary, objects are identical")
		return nil
	}

	_, err = c.client.ClusterOperators().UpdateStatus(ctx, updated, metav1.UpdateOptions{})
	return err
}

// update the cluster controller status conditions
func updateControllerConditions(cs *conditions, ctrlStatus *controllerStatus,
	isInitializing bool, lastTransition time.Time) {
	if isInitializing {
		// the disabled condition is optional, but set it now if we already know we're disabled
		if ds := ctrlStatus.getStatus(DisabledStatus); ds != nil {
			cs.setCondition(OperatorDisabled, configv1.ConditionTrue, ds.reason, ds.message, metav1.Now())
		}
		if !cs.hasCondition(configv1.OperatorDegraded) {
			cs.setCondition(configv1.OperatorDegraded, configv1.ConditionFalse, "AsExpected", "", metav1.Now())
		}
	}

	// once we've initialized set Failing and Disabled as best we know
	// handle when disabled
	if ds := ctrlStatus.getStatus(DisabledStatus); ds != nil {
		cs.setCondition(OperatorDisabled, configv1.ConditionTrue, ds.reason, ds.message, metav1.Now())
	} else {
		cs.setCondition(OperatorDisabled, configv1.ConditionFalse, "AsExpected", "", metav1.Now())
	}

	// handle when has errors
	if es := ctrlStatus.getStatus(ErrorStatus); es != nil {
		cs.setCondition(configv1.OperatorDegraded, configv1.ConditionTrue, es.reason, es.message, metav1.Time{Time: lastTransition})
	} else {
		cs.setCondition(configv1.OperatorDegraded, configv1.ConditionFalse, "AsExpected", "", metav1.Now())
	}

	// handle when upload fails
	if ur := ctrlStatus.getStatus(UploadStatus); ur != nil {
		cs.setCondition(InsightsUploadDegraded, configv1.ConditionTrue, ur.reason, ur.message, metav1.Time{Time: lastTransition})
	} else {
		cs.removeCondition(InsightsUploadDegraded)
	}

	// handle when download fails
	if ds := ctrlStatus.getStatus(DownloadStatus); ds != nil {
		cs.setCondition(InsightsDownloadDegraded, configv1.ConditionTrue, ds.reason, ds.message, metav1.Time{Time: lastTransition})
	} else {
		cs.removeCondition(InsightsDownloadDegraded)
	}
}

// update the current controller state by it status
func updateControllerConditionsByStatus(cs *conditions, ctrlStatus *controllerStatus,
	isInitializing bool, lastTransition time.Time) {
	if isInitializing {
		klog.V(4).Infof("The operator is still being initialized")
		// if we're still starting up and some sources are not ready, initialize the conditions
		// but don't update
		if !cs.hasCondition(configv1.OperatorProgressing) {
			cs.setCondition(configv1.OperatorProgressing, configv1.ConditionTrue, "Initializing", "Initializing the operator", metav1.Now())
		}
	}

	if es := ctrlStatus.getStatus(ErrorStatus); es != nil {
		klog.V(4).Infof("The operator has some internal errors: %s", es.message)
		cs.setCondition(configv1.OperatorProgressing, configv1.ConditionFalse, "Degraded", "An error has occurred", metav1.Now())
	}

	if ds := ctrlStatus.getStatus(DisabledStatus); ds != nil {
		klog.V(4).Infof("The operator is marked as disabled")
		cs.setCondition(configv1.OperatorProgressing, configv1.ConditionFalse, ds.reason, ds.message, metav1.Time{Time: lastTransition})
	}

	if ctrlStatus.isHealthy() {
		klog.V(4).Infof("The operator is healthy")
		cs.setCondition(configv1.OperatorProgressing, configv1.ConditionFalse, "AsExpected", "Monitoring the cluster", metav1.Now())
	}
}

// handle the controller status error by formatting the present errors messages when needed
func handleControllerStatusError(errs []string, errorReason string) (reason, message string) {
	if len(errs) > 1 {
		reason = "MultipleFailures"
		sort.Strings(errs)
		message = fmt.Sprintf("There are multiple errors blocking progress:\n* %s", strings.Join(errs, "\n* "))
	} else if len(errs) == 1 {
		if len(errorReason) == 0 {
			reason = "UnknownError"
		}
		message = errs[0]
	}
	return reason, message
}

// create a new cluster operator with defaults values
func newClusterOperator(name string, status *configv1.ClusterOperatorStatus) *configv1.ClusterOperator {
	co := &configv1.ClusterOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	if status != nil {
		co.Status = *status
	}

	return co
}

// create the operator's related objects
func relatedObjects(namespace string) []configv1.ObjectReference {
	return []configv1.ObjectReference{
		{Resource: "namespaces", Name: namespace},
		{Group: "apps", Resource: "deployments", Namespace: namespace, Name: "insights-operator"},
		{Resource: "secrets", Namespace: "openshift-config", Name: "pull-secret"},
		{Resource: "secrets", Namespace: "openshift-config", Name: "support"},
		{Resource: "serviceaccounts", Namespace: namespace, Name: "gather"},
		{Resource: "serviceaccounts", Namespace: namespace, Name: "operator"},
		{Resource: "services", Namespace: namespace, Name: "metrics"},
		{Resource: "configmaps", Namespace: namespace, Name: "service-ca-bundle"},
	}
}
