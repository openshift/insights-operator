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
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"
)

const (
	// OperatorDisabled defines the condition type when the operator's primary funcion has been disabled
	OperatorDisabled configv1.ClusterStatusConditionType = "Disabled"
	// UploadDegraded defines the condition type (when set to True) when an archive can't be successfully uploaded
	UploadDegraded configv1.ClusterStatusConditionType = "UploadDegraded"
	// InsightsDownloadDegraded defines the condition type (when set to True) when a Insights report can't be successfully downloaded
	InsightsDownloadDegraded configv1.ClusterStatusConditionType = "InsightsDownloadDegraded"
	// How many upload failures in a row we tolerate before starting reporting
	// as UploadDegraded
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

// Controller is the type responsible for managing the status of the operator according to the status of the sources.
// Sources come from different major parts of the codebase, for the purpose of communicating their status with the controller.
type Controller struct {
	name         string
	namespace    string
	client       configv1client.ConfigV1Interface
	coreClient   corev1client.CoreV1Interface
	statusCh     chan struct{}
	configurator Configurator

	lock     sync.Mutex
	sources  []controllerstatus.Interface
	reported Reported
	start    time.Time
}

// NewController creates a status controller, responsible for monitoring the operators status and updating the it's cluster status accordingly.
func NewController(client configv1client.ConfigV1Interface,
	coreClient corev1client.CoreV1Interface,
	configurator Configurator, namespace string) *Controller {
	c := &Controller{
		name:         "insights",
		client:       client,
		coreClient:   coreClient,
		statusCh:     make(chan struct{}, 1),
		configurator: configurator,
		namespace:    namespace,
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

func (c *Controller) merge(existing *configv1.ClusterOperator) *configv1.ClusterOperator { //nolint: funlen, gocyclo
	// prime the object if it does not exist
	if existing == nil {
		existing = &configv1.ClusterOperator{
			ObjectMeta: metav1.ObjectMeta{
				Name: c.name,
			},
		}
	}

	// calculate the current controller state
	var last time.Time
	var errorReason string
	var errs []string
	var uploadErrorReason,
		uploadErrorMessage,
		disabledReason,
		disabledMessage,
		downloadReason,
		downloadMessage string

	allReady := true

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
			klog.Errorf("Programmer error: status source %d %T reported an empty message: %#v", i, source, summary)
			continue
		}

		degradingFailure := false

		if summary.Operation == controllerstatus.Uploading {
			if summary.Count < uploadFailuresCountThreshold {
				klog.V(4).Infof("Number of last upload failures %d lower than threshold %d. Not marking as degraded.",
					summary.Count, uploadFailuresCountThreshold)
			} else {
				degradingFailure = true
				klog.V(4).Infof("Number of last upload failures %d exceeded than threshold %d. Marking as degraded.",
					summary.Count, uploadFailuresCountThreshold)
			}
			uploadErrorReason = summary.Reason
			uploadErrorMessage = summary.Message
		} else if summary.Operation == controllerstatus.DownloadingReport {
			klog.V(4).Info("Failed to download Insights report")
			downloadReason = summary.Reason
			downloadMessage = summary.Message
		} else if summary.Operation == controllerstatus.PullingSCACerts {
			klog.V(4).Infof("Failed to download the SCA certs within the threshold %d with exponential backoff. Marking as degraded.",
				OCMAPIFailureCountThreshold)
			degradingFailure = true
		}

		if degradingFailure {
			errorReason = summary.Reason
			errs = append(errs, summary.Message)
		}

		if last.Before(summary.LastTransitionTime) {
			last = summary.LastTransitionTime
		}
	}

	var errorMessage string

	switch len(errs) {
	case 0:
	case 1:
		if len(errorReason) == 0 {
			errorReason = "UnknownError"
		}
		errorMessage = errs[0]
	default:
		errorReason = "MultipleFailures"
		sort.Strings(errs)
		errorMessage = fmt.Sprintf("There are multiple errors blocking progress:\n* %s", strings.Join(errs, "\n* "))
	}
	// disabled state only when it's disabled by config. It means that gathering will not happen
	if !c.configurator.Config().Report {
		disabledReason = "Disabled"
		disabledMessage = "Health reporting is disabled"
	}

	existing = existing.DeepCopy()
	now := time.Now()
	if len(c.namespace) > 0 {
		existing.Status.RelatedObjects = []configv1.ObjectReference{
			{Resource: "namespaces", Name: c.namespace},
			{Group: "apps", Resource: "deployments", Namespace: c.namespace, Name: "insights-operator"},
			{Resource: "secrets", Namespace: "openshift-config", Name: "pull-secret"},
			{Resource: "secrets", Namespace: "openshift-config", Name: "support"},
			{Resource: "serviceaccounts", Namespace: c.namespace, Name: "gather"},
			{Resource: "serviceaccounts", Namespace: c.namespace, Name: "operator"},
			{Resource: "services", Namespace: c.namespace, Name: "metrics"},
			{Resource: "configmaps", Namespace: c.namespace, Name: "service-ca-bundle"},
		}
	}
	reported := Reported{LastReportTime: metav1.Time{Time: c.LastReportedTime()}}
	isInitializing := !allReady && now.Sub(c.controllerStartTime()) < 3*time.Minute

	// update the disabled and failing conditions
	switch {
	case isInitializing:
		// the disabled condition is optional, but set it now if we already know we're disabled
		if len(disabledReason) > 0 {
			setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
				Type:    OperatorDisabled,
				Status:  configv1.ConditionTrue,
				Reason:  disabledReason,
				Message: disabledMessage,
			})
		}

		if findOperatorStatusCondition(existing.Status.Conditions, configv1.OperatorDegraded) == nil {
			setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
				Type:   configv1.OperatorDegraded,
				Status: configv1.ConditionFalse,
				Reason: "AsExpected",
			})
		}

	default:
		// once we've initialized set Failing and Disabled as best we know
		setDisabledOperatorStatusCondition(existing, disabledReason, disabledMessage)
		setDegradedOperatorStatusCondition(existing, last, errorReason, errorMessage)
		setUploadDegradedOperatorStatusCondition(existing, last, uploadErrorReason, uploadErrorMessage)
		setInsightsDownloadDegradedOperatorStatusCondition(existing, last, downloadReason, downloadMessage)
	}

	// once the operator is running it is always considered available
	setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
		Type:   configv1.OperatorAvailable,
		Status: configv1.ConditionTrue,
		Reason: "AsExpected",
	})

	// update the Progressing condition with a summary of the current state
	switch {
	case isInitializing:
		klog.V(4).Infof("The operator is still being initialized")
		// if we're still starting up and some sources are not ready, initialize the conditions
		// but don't update
		if findOperatorStatusCondition(existing.Status.Conditions, configv1.OperatorProgressing) == nil {
			setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
				Type:    configv1.OperatorProgressing,
				Status:  configv1.ConditionTrue,
				Reason:  "Initializing",
				Message: "Initializing the operator",
			})
		}

	case len(errorMessage) > 0:
		klog.V(4).Infof("The operator has some internal errors: %s", errorMessage)
		setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
			Type:    configv1.OperatorProgressing,
			Status:  configv1.ConditionFalse,
			Reason:  "Degraded",
			Message: "An error has occurred",
		})

	case len(disabledMessage) > 0:
		klog.V(4).Infof("The operator is marked as disabled")
		setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
			Type:               configv1.OperatorProgressing,
			Status:             configv1.ConditionFalse,
			LastTransitionTime: metav1.Time{Time: last},
			Reason:             errorReason,
			Message:            disabledMessage,
		})

	default:
		klog.V(4).Infof("The operator is healthy")
		setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
			Type:    configv1.OperatorProgressing,
			Status:  configv1.ConditionFalse,
			Reason:  "AsExpected",
			Message: "Monitoring the cluster",
		})
	}

	if release := os.Getenv("RELEASE_VERSION"); len(release) > 0 {
		existing.Status.Versions = []configv1.OperandVersion{
			{Name: "operator", Version: release},
		}
	}

	if data, err := json.Marshal(reported); err != nil {
		klog.Errorf("Unable to marshal status extension: %v", err)
	} else {
		existing.Status.Extension.Raw = data
	}
	return existing
}

func setInsightsDownloadDegradedOperatorStatusCondition(existing *configv1.ClusterOperator, last time.Time, downloadReason string, downloadMessage string) {
	if len(downloadReason) > 0 {
		setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
			Type:               InsightsDownloadDegraded,
			Status:             configv1.ConditionTrue,
			LastTransitionTime: metav1.Time{Time: last},
			Reason:             downloadReason,
			Message:            downloadMessage,
		})
	} else {
		removeOperatorStatusCondition(&existing.Status.Conditions, InsightsDownloadDegraded)
	}
}

func setUploadDegradedOperatorStatusCondition(existing *configv1.ClusterOperator, last time.Time, uploadErrorReason string, uploadErrorMessage string) {
	if len(uploadErrorReason) > 0 {
		setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
			Type:               UploadDegraded,
			Status:             configv1.ConditionTrue,
			LastTransitionTime: metav1.Time{Time: last},
			Reason:             uploadErrorReason,
			Message:            uploadErrorMessage,
		})
	} else {
		removeOperatorStatusCondition(&existing.Status.Conditions, UploadDegraded)
	}
}

func setDegradedOperatorStatusCondition(existing *configv1.ClusterOperator, last time.Time, errorReason string, errorMessage string,) {
	if len(errorMessage) > 0 {
		klog.V(4).Infof("The operator has some internal errors: %s", errorMessage)
		setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
			Type:               configv1.OperatorDegraded,
			Status:             configv1.ConditionTrue,
			LastTransitionTime: metav1.Time{Time: last},
			Reason:             errorReason,
			Message:            errorMessage,
		})
	} else {
		setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
			Type:   configv1.OperatorDegraded,
			Status: configv1.ConditionFalse,
			Reason: "AsExpected",
		})
	}
}

func setDisabledOperatorStatusCondition(existing *configv1.ClusterOperator, disabledMessage string, disabledReason string) {
	if len(disabledMessage) > 0 {
		setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
			Type:    OperatorDisabled,
			Status:  configv1.ConditionTrue,
			Reason:  disabledReason,
			Message: disabledMessage,
		})
	} else {
		setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
			Type:   OperatorDisabled,
			Status: configv1.ConditionFalse,
			Reason: "AsExpected",
		})
	}
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
			if con := findOperatorStatusCondition(existing.Status.Conditions, configv1.OperatorDegraded); con == nil ||
				con != nil && con.Status == configv1.ConditionFalse {
				klog.Info("The initial operator extension status is healthy")
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
		klog.V(4).Infof("No status update necessary, objects are identical")
		return nil
	}

	_, err = c.client.ClusterOperators().UpdateStatus(ctx, updated, metav1.UpdateOptions{})
	return err
}

func setOperatorStatusCondition(conditions *[]configv1.ClusterOperatorStatusCondition,
	newCondition configv1.ClusterOperatorStatusCondition) { //nolint: gocritic
	if conditions == nil {
		conditions = &[]configv1.ClusterOperatorStatusCondition{}
	}
	existingCondition := findOperatorStatusCondition(*conditions, newCondition.Type)
	if existingCondition == nil {
		newCondition.LastTransitionTime = metav1.NewTime(time.Now())
		*conditions = append(*conditions, newCondition)
		return
	}

	if existingCondition.Status != newCondition.Status {
		existingCondition.Status = newCondition.Status
		existingCondition.LastTransitionTime = newCondition.LastTransitionTime
	}

	existingCondition.Reason = newCondition.Reason
	existingCondition.Message = newCondition.Message

	if existingCondition.LastTransitionTime.IsZero() {
		existingCondition.LastTransitionTime = metav1.NewTime(time.Now())
	}
}

func removeOperatorStatusCondition(conditions *[]configv1.ClusterOperatorStatusCondition,
	conditionType configv1.ClusterStatusConditionType) {
	if conditions == nil {
		return
	}
	var newConditions []configv1.ClusterOperatorStatusCondition
	for _, condition := range *conditions {
		if condition.Type != conditionType {
			newConditions = append(newConditions, condition)
		}
	}

	*conditions = newConditions
}

func findOperatorStatusCondition(conditions []configv1.ClusterOperatorStatusCondition,
	conditionType configv1.ClusterStatusConditionType) *configv1.ClusterOperatorStatusCondition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
