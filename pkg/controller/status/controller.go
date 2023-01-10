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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/controllerstatus"
	"github.com/openshift/insights-operator/pkg/ocm"
	"github.com/openshift/insights-operator/pkg/ocm/clustertransfer"
	"github.com/openshift/insights-operator/pkg/ocm/sca"
)

const (
	// How many upload failures in a row we tolerate before starting reporting
	// as InsightsUploadDegraded
	uploadFailuresCountThreshold = 5

	asExpectedReason         = "AsExpected"
	degradedReason           = "Degraded"
	noTokenReason            = "NoToken"
	upgradeableReason        = "InsightsUpgradeable"
	insightsAvailableMessage = "Insights works as expected"
	reportingDisabledMsg     = "Health reporting is disabled"
	monitoringMsg            = "Monitoring the cluster"
	canBeUpgradedMsg         = "Insights operator can be upgraded"
)

type Reported struct {
	LastReportTime metav1.Time `json:"lastReportTime"`
}

// Controller is the type responsible for managing the statusMessage of the operator according to the statusMessage of the sources.
// Sources come from different major parts of the codebase, for the purpose of communicating their statusMessage with the controller.
type Controller struct {
	name      string
	namespace string

	client configv1client.ConfigV1Interface

	statusCh           chan struct{}
	secretConfigurator configobserver.Configurator
	apiConfigurator    configobserver.APIConfigObserver

	sources  map[string]controllerstatus.StatusController
	reported Reported
	start    time.Time

	ctrlStatus *controllerStatus

	lock sync.Mutex
}

// NewController creates a statusMessage controller, responsible for monitoring the operators statusMessage and updating its cluster statusMessage accordingly.
func NewController(client configv1client.ConfigV1Interface,
	secretConfigurator configobserver.Configurator,
	apiConfigurator configobserver.APIConfigObserver,
	namespace string) *Controller {
	c := &Controller{
		name:               "insights",
		statusCh:           make(chan struct{}, 1),
		secretConfigurator: secretConfigurator,
		apiConfigurator:    apiConfigurator,
		client:             client,
		namespace:          namespace,
		sources:            make(map[string]controllerstatus.StatusController),
		ctrlStatus:         newControllerStatus(),
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
func (c *Controller) AddSources(sources ...controllerstatus.StatusController) {
	c.lock.Lock()
	defer c.lock.Unlock()
	for i := range sources {
		source := sources[i]
		c.sources[source.Name()] = source
	}
}

// Sources provides the sources in a thread-safe way.
// A source is used to monitor parts of the operator.
func (c *Controller) Sources() map[string]controllerstatus.StatusController {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.sources
}

func (c *Controller) Source(name string) controllerstatus.StatusController {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.sources[name]
}

func (c *Controller) merge(clusterOperator *configv1.ClusterOperator) *configv1.ClusterOperator {
	// prime the object if it does not exist
	if clusterOperator == nil {
		clusterOperator = newClusterOperator(c.name, nil)
	}

	// make sure to start a clean status controller
	c.ctrlStatus.reset()

	// calculate the current controller state
	allReady := c.currentControllerStatus()

	clusterOperator = clusterOperator.DeepCopy()
	now := time.Now()
	if len(c.namespace) > 0 {
		clusterOperator.Status.RelatedObjects = relatedObjects(c.namespace)
	}

	isInitializing := !allReady && now.Sub(c.controllerStartTime()) < 3*time.Minute

	// cluster operator conditions
	cs := newConditions(&clusterOperator.Status, metav1.Time{Time: now})
	c.updateControllerConditions(cs, isInitializing)
	c.updateControllerConditionsByStatus(cs, isInitializing)

	// all status conditions from conditions to cluster operator
	clusterOperator.Status.Conditions = cs.entries()

	if release := os.Getenv("RELEASE_VERSION"); len(release) > 0 {
		clusterOperator.Status.Versions = []configv1.OperandVersion{
			{Name: "operator", Version: release},
		}
	}

	reported := Reported{LastReportTime: metav1.Time{Time: c.LastReportedTime()}}
	if data, err := json.Marshal(reported); err != nil {
		klog.Errorf("Unable to marshal status extension: %v", err)
	} else {
		clusterOperator.Status.Extension.Raw = data
	}
	return clusterOperator
}

// calculate the current controller status based on its given sources
func (c *Controller) currentControllerStatus() (allReady bool) { //nolint: gocyclo
	var errorReason string
	var errs []string

	allReady = true

	for name, source := range c.Sources() {
		summary, ready := source.CurrentStatus()
		if !ready {
			klog.V(4).Infof("Source %s %T is not ready", name, source)
			allReady = false
			continue
		}
		if summary.Healthy {
			continue
		}
		if len(summary.Message) == 0 {
			klog.Errorf("Programmer error: status source %s %T reported an empty message: %#v", name, source, summary)
			continue
		}

		degradingFailure := false

		if summary.Operation.Name == controllerstatus.Uploading.Name {
			if summary.Count < uploadFailuresCountThreshold {
				klog.V(4).Infof("Number of last upload failures %d lower than threshold %d. Not marking as degraded.",
					summary.Count, uploadFailuresCountThreshold)
			} else {
				degradingFailure = true
				klog.V(4).Infof("Number of last upload failures %d exceeded the threshold %d. Marking as degraded.",
					summary.Count, uploadFailuresCountThreshold)
			}
			c.ctrlStatus.setStatus(UploadStatus, summary.Reason, summary.Message)
		} else if summary.Operation.Name == controllerstatus.DownloadingReport.Name {
			klog.V(4).Info("Failed to download Insights report")
			c.ctrlStatus.setStatus(DownloadStatus, summary.Reason, summary.Message)
		} else if summary.Operation.Name == controllerstatus.PullingSCACerts.Name {
			// mark as degraded only in case of HTTP 500 and higher
			if summary.Operation.HTTPStatusCode >= 500 {
				klog.V(4).Infof("Failed to download the SCA certs within the threshold %d with exponential backoff. Marking as degraded.",
					ocm.OCMAPIFailureCountThreshold)
				degradingFailure = true
			}
		} else if summary.Operation.Name == controllerstatus.PullingClusterTransfer.Name {
			// mark as degraded only in case of HTTP 500 and higher
			if summary.Operation.HTTPStatusCode >= 500 {
				klog.V(4).Infof("Failed to pull the cluster transfer object within the threshold %d with exponential backoff. Marking as degraded.",
					ocm.OCMAPIFailureCountThreshold)
				degradingFailure = true
			}
		}

		if degradingFailure {
			errorReason = summary.Reason
			errs = append(errs, summary.Message)
		}
	}

	// handling errors
	errorReason, errorMessage := handleControllerStatusError(errs, errorReason)
	if errorReason != "" || errorMessage != "" {
		c.ctrlStatus.setStatus(ErrorStatus, errorReason, errorMessage)
	}

	c.checkDisabledGathering()

	return allReady
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
					klog.Errorf("Limiter error by status: %v", err)
				}
			}
			if err := c.updateStatus(ctx, false); err != nil {
				klog.Errorf("Unable to write cluster operator status: %v", err)
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
					klog.Errorf("The initial operator extension status is invalid: %v", err)
				}
			}
			c.SetLastReportedTime(reported.LastReportTime.Time.UTC())
			cs := newConditions(&existing.Status, metav1.Now())
			if con := cs.findCondition(configv1.OperatorDegraded); con == nil ||
				con != nil && con.Status == configv1.ConditionFalse {
				klog.Info("The initial operator extension status is healthy")
			}
		}
	}

	updatedClusterOperator := c.merge(existing)
	if existing == nil {
		created, err := c.client.ClusterOperators().Create(ctx, updatedClusterOperator, metav1.CreateOptions{}) //nolint: govet
		if err != nil {
			return err
		}
		updatedClusterOperator.ObjectMeta = created.ObjectMeta
		updatedClusterOperator.Spec = created.Spec
	} else if reflect.DeepEqual(updatedClusterOperator.Status, existing.Status) {
		klog.V(4).Infof("No status update necessary, objects are identical")
		return nil
	}
	_, err = c.client.ClusterOperators().UpdateStatus(ctx, updatedClusterOperator, metav1.UpdateOptions{})
	return err
}

// update the cluster controller status conditions
func (c *Controller) updateControllerConditions(cs *conditions, isInitializing bool) {
	if isInitializing {
		// the disabled condition is optional, but set it now if we already know we're disabled
		if ds := c.ctrlStatus.getStatus(DisabledStatus); ds != nil {
			cs.setCondition(OperatorDisabled, configv1.ConditionTrue, ds.reason, ds.message)
		}
		if !cs.hasCondition(configv1.OperatorDegraded) {
			cs.setCondition(configv1.OperatorDegraded, configv1.ConditionFalse, asExpectedReason, "")
		}
	}

	// once we've initialized set Failing and Disabled as best we know
	// handle when disabled
	if ds := c.ctrlStatus.getStatus(DisabledStatus); ds != nil {
		cs.setCondition(OperatorDisabled, configv1.ConditionTrue, ds.reason, ds.message)
	} else {
		cs.setCondition(OperatorDisabled, configv1.ConditionFalse, asExpectedReason, "")
	}
	// handle when has errors
	if es := c.ctrlStatus.getStatus(ErrorStatus); es != nil && !c.ctrlStatus.isDisabled() {
		cs.setCondition(configv1.OperatorDegraded, configv1.ConditionTrue, es.reason, es.message)
	} else {
		cs.setCondition(configv1.OperatorDegraded, configv1.ConditionFalse, asExpectedReason, insightsAvailableMessage)
	}

	// handle when upload fails
	if ur := c.ctrlStatus.getStatus(UploadStatus); ur != nil && !c.ctrlStatus.isDisabled() {
		cs.setCondition(InsightsUploadDegraded, configv1.ConditionTrue, ur.reason, ur.message)
	} else {
		cs.removeCondition(InsightsUploadDegraded)
	}

	// handle when download fails
	if ds := c.ctrlStatus.getStatus(DownloadStatus); ds != nil && !c.ctrlStatus.isDisabled() {
		cs.setCondition(InsightsDownloadDegraded, configv1.ConditionTrue, ds.reason, ds.message)
	} else {
		cs.removeCondition(InsightsDownloadDegraded)
	}
	c.updateControllerConditionByReason(cs, SCAAvailable, sca.ControllerName, sca.AvailableReason, isInitializing)
	c.updateControllerConditionByReason(cs,
		ClusterTransferAvailable,
		clustertransfer.ControllerName,
		clustertransfer.AvailableReason,
		isInitializing)
}

func (c *Controller) updateControllerConditionByReason(cs *conditions,
	condition configv1.ClusterStatusConditionType,
	controllerName, reason string,
	isInitializing bool) {
	controller := c.Source(controllerName)
	if controller == nil {
		return
	}
	if isInitializing {
		return
	}
	summary, ok := controller.CurrentStatus()
	// no summary to read
	if !ok {
		return
	}
	if summary.Reason == reason {
		cs.setCondition(condition, configv1.ConditionTrue, summary.Reason, summary.Message)
	} else {
		cs.setCondition(condition, configv1.ConditionFalse, summary.Reason, summary.Message)
	}
}

func (c *Controller) checkDisabledGathering() {
	// disabled state only when it's disabled by config. It means that gathering will not happen
	if !c.secretConfigurator.Config().Report {
		c.ctrlStatus.setStatus(DisabledStatus, noTokenReason, reportingDisabledMsg)
	}

	// check if the gathering is disabled in the `insightsdatagather.config.openshift.io` API
	if c.apiConfigurator != nil {
		if c.apiConfigurator.GatherDisabled() {
			c.ctrlStatus.setStatus(DisabledStatus, "DisabledInAPI", reportingDisabledMsg)
		}
	}
}

// update the current controller state by it status
func (c *Controller) updateControllerConditionsByStatus(cs *conditions, isInitializing bool) {
	if isInitializing {
		klog.V(4).Infof("The operator is still being initialized")
		// if we're still starting up and some sources are not ready, initialize the conditions
		// but don't update
		if !cs.hasCondition(configv1.OperatorProgressing) {
			cs.setCondition(configv1.OperatorProgressing, configv1.ConditionTrue, "Initializing", "Initializing the operator")
		}
	}

	if es := c.ctrlStatus.getStatus(ErrorStatus); es != nil && !c.ctrlStatus.isDisabled() {
		klog.V(4).Infof("The operator has some internal errors: %s", es.message)
		cs.setCondition(configv1.OperatorProgressing, configv1.ConditionFalse, degradedReason, "An error has occurred")
		cs.setCondition(configv1.OperatorAvailable, configv1.ConditionFalse, es.reason, es.message)
		cs.setCondition(configv1.OperatorUpgradeable, configv1.ConditionFalse, degradedReason, es.message)
	}

	// when the operator is already healthy then it doesn't make sense to set those, but when it's degraded and then
	// marked as disabled then it's reasonable to set Available=True
	if ds := c.ctrlStatus.getStatus(DisabledStatus); ds != nil && !c.ctrlStatus.isHealthy() {
		klog.V(4).Infof("The operator is marked as disabled")
		cs.setCondition(configv1.OperatorProgressing, configv1.ConditionFalse, asExpectedReason, monitoringMsg)
		cs.setCondition(configv1.OperatorAvailable, configv1.ConditionTrue, asExpectedReason, insightsAvailableMessage)
		cs.setCondition(configv1.OperatorUpgradeable, configv1.ConditionTrue, upgradeableReason, canBeUpgradedMsg)
	}

	if c.ctrlStatus.isHealthy() {
		klog.V(4).Infof("The operator is healthy")
		cs.setCondition(configv1.OperatorProgressing, configv1.ConditionFalse, asExpectedReason, monitoringMsg)
		cs.setCondition(configv1.OperatorAvailable, configv1.ConditionTrue, asExpectedReason, insightsAvailableMessage)
		cs.setCondition(configv1.OperatorUpgradeable, configv1.ConditionTrue, upgradeableReason, canBeUpgradedMsg)
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
		reason = errorReason
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
		{Group: "operator.openshift.io", Resource: "insightsoperators", Name: "cluster"},
	}
}
