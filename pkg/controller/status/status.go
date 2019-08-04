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

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/openshift/support-operator/pkg/config"
	"github.com/openshift/support-operator/pkg/controllerstatus"
)

type Reported struct {
	LastReportTime metav1.Time `json:"lastReportTime"`
}

type Configurator interface {
	Config() *config.Controller
}

type Controller struct {
	name         string
	namespace    string
	client       configv1client.ConfigV1Interface
	statusCh     chan struct{}
	configurator Configurator

	lock     sync.Mutex
	sources  []controllerstatus.Interface
	reported Reported
	start    time.Time
}

func NewController(client configv1client.ConfigV1Interface, configurator Configurator, namespace string) *Controller {
	c := &Controller{
		name:         "insights",
		client:       client,
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

func (c *Controller) LastReportedTime() time.Time {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.reported.LastReportTime.Time
}

func (c *Controller) SetLastReportedTime(at time.Time) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.reported.LastReportTime.IsZero() {
		klog.V(2).Infof("Initializing last reported time to %s", at.UTC().Format(time.RFC3339))
	}
	c.reported.LastReportTime.Time = at
	c.triggerStatusUpdate()
}

func (c *Controller) AddSources(sources ...controllerstatus.Interface) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.sources = append(c.sources, sources...)
}

func (c *Controller) Sources() []controllerstatus.Interface {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.sources
}

func (c *Controller) merge(existing *configv1.ClusterOperator) *configv1.ClusterOperator {
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
	var reason string
	var errors []string
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
		reason = summary.Reason
		errors = append(errors, summary.Message)
		if last.Before(summary.LastTransitionTime) {
			last = summary.LastTransitionTime
		}
	}
	var errorMessage string
	switch len(errors) {
	case 0:
	case 1:
		if len(reason) == 0 {
			reason = "UnknownError"
		}
		errorMessage = errors[0]
	default:
		reason = "MultipleFailures"
		sort.Strings(errors)
		errorMessage = fmt.Sprintf("There are multiple errors blocking progress:\n* %s", strings.Join(errors, "\n* "))
	}
	var disabledMessage string
	if !c.configurator.Config().Report {
		disabledMessage = "Health reporting is disabled"
	}
	// NotAuthorized is a special case where we want to disable the operator
	if reason == "NotAuthorized" {
		disabledMessage = errorMessage
		errorMessage = ""
	}

	existing = existing.DeepCopy()
	now := time.Now()
	if len(c.namespace) > 0 {
		existing.Status.RelatedObjects = []configv1.ObjectReference{
			{Resource: "namespaces", Name: c.namespace},
			{Group: "apps", Resource: "deployments", Namespace: c.namespace, Name: "insights-operator"},
		}
	}
	reported := Reported{LastReportTime: metav1.Time{Time: c.LastReportedTime()}}
	isInitializing := !allReady && now.Sub(c.controllerStartTime()) < 3*time.Minute

	// update the disabled and failing conditions
	switch {
	case isInitializing:
		// the disabled condition is optional, but set it now if we already know we're disabled
		if len(disabledMessage) > 0 {
			setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
				Type:    OperatorDisabled,
				Status:  configv1.ConditionTrue,
				Reason:  reason,
				Message: disabledMessage,
			})
		}

		if findOperatorStatusCondition(existing.Status.Conditions, configv1.OperatorDegraded) == nil {
			setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
				Type:   configv1.OperatorDegraded,
				Status: configv1.ConditionFalse,
			})
		}

	default:
		// once we've initialized set Failing and Disabled as best we know
		if len(disabledMessage) > 0 {
			setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
				Type:    OperatorDisabled,
				Status:  configv1.ConditionTrue,
				Reason:  reason,
				Message: disabledMessage,
			})
		} else {
			setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
				Type:   OperatorDisabled,
				Status: configv1.ConditionFalse,
			})
		}

		if len(errorMessage) > 0 {
			setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
				Type:               configv1.OperatorDegraded,
				Status:             configv1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: last},
				Reason:             reason,
				Message:            errorMessage,
			})
		} else {
			setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
				Type:   configv1.OperatorDegraded,
				Status: configv1.ConditionFalse,
			})
		}
	}

	// once the operator is running it is always considered available
	setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
		Type:   configv1.OperatorAvailable,
		Status: configv1.ConditionTrue,
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
				Message: "Initializing the operator",
			})
		}

	case len(errorMessage) > 0:
		klog.V(4).Infof("The operator has some internal errors: %s", errorMessage)
		setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
			Type:    configv1.OperatorProgressing,
			Status:  configv1.ConditionFalse,
			Message: "An error has occurred",
		})

	case len(disabledMessage) > 0:
		klog.V(4).Infof("The operator is marked as disabled")
		setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
			Type:               configv1.OperatorProgressing,
			Status:             configv1.ConditionFalse,
			LastTransitionTime: metav1.Time{Time: last},
			Reason:             reason,
			Message:            disabledMessage,
		})

	default:
		klog.V(4).Infof("The operator is healthy")
		setOperatorStatusCondition(&existing.Status.Conditions, configv1.ClusterOperatorStatusCondition{
			Type:    configv1.OperatorProgressing,
			Status:  configv1.ConditionFalse,
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

func (c *Controller) Start(ctx context.Context) error {
	if err := c.updateStatus(true); err != nil {
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
				limiter.Wait(ctx)
			case <-c.statusCh:
				limiter.Wait(ctx)
			}
			if err := c.updateStatus(false); err != nil {
				klog.Errorf("Unable to write cluster operator status: %v", err)
			}
		}

	}, time.Second, ctx.Done())
	return nil
}

func (c *Controller) updateStatus(initial bool) error {
	existing, err := c.client.ClusterOperators().Get(c.name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		existing = nil
	}
	if initial && existing != nil {
		var reported Reported
		if len(existing.Status.Extension.Raw) > 0 {
			if err := json.Unmarshal(existing.Status.Extension.Raw, &reported); err != nil {
				klog.Errorf("The initial operator extension status is invalid: %v", err)
			}
		}
		c.SetLastReportedTime(reported.LastReportTime.Time.UTC())
	}

	updated := c.merge(existing)
	if existing == nil {
		created, err := c.client.ClusterOperators().Create(updated)
		if err != nil {
			return err
		}
		updated.ObjectMeta = created.ObjectMeta
		updated.Spec = created.Spec
	} else {
		if reflect.DeepEqual(updated.Status, existing.Status) {
			klog.V(4).Infof("No status update necessary, objects are identical")
			return nil
		}
	}

	_, err = c.client.ClusterOperators().UpdateStatus(updated)
	return err
}

// OperatorDisabled reports when the primary function of the operator has been disabled.
const OperatorDisabled configv1.ClusterStatusConditionType = "Disabled"

func setOperatorStatusCondition(conditions *[]configv1.ClusterOperatorStatusCondition, newCondition configv1.ClusterOperatorStatusCondition) {
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

func findOperatorStatusCondition(conditions []configv1.ClusterOperatorStatusCondition, conditionType configv1.ClusterStatusConditionType) *configv1.ClusterOperatorStatusCondition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
