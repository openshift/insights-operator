package clustertransfer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/controllerstatus"
	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
	"github.com/openshift/insights-operator/pkg/ocm"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"
)

const (
	ControllerName  = "clusterTransferController"
	AvailableReason = "PullSecretUpdated"
)

var (
	disconnectedReason           = "Disconnected"
	noClusterTransfer            = "NoClusterTransfer"
	moreAcceptedClusterTransfers = "MoreAcceptedClusterTransfers"
	dataCorrupted                = "DataCorrupted"
	unexpectedData               = "UnexpectedData"
)

type Controller struct {
	controllerstatus.StatusController
	coreClient   corev1client.CoreV1Interface
	configurator configobserver.Interface
	client       clusterTransferClient
	pullSecret   *v1.Secret
}

type clusterTransferClient interface {
	RecvClusterTransfer(endpoint string) ([]byte, error)
}

// New creates new instance of the cluster transfer controller
func New(coreClient corev1client.CoreV1Interface,
	configurator configobserver.Interface,
	insightsClient clusterTransferClient) *Controller {
	return &Controller{
		StatusController: controllerstatus.New(ControllerName),
		coreClient:       coreClient,
		configurator:     configurator,
		client:           insightsClient,
	}
}

// Run periodically queries the OCM API and update pull-secret accordingly
func (c *Controller) Run(ctx context.Context) {
	cfg := c.configurator.Config()
	endpoint := cfg.ClusterTransfer.Endpoint
	interval := cfg.ClusterTransfer.Interval
	configCh, cancel := c.configurator.ConfigChanged()
	defer cancel()
	c.requestDataAndUpdateSecret(ctx, endpoint)
	for {
		select {
		case <-time.After(interval):
			c.requestDataAndUpdateSecret(ctx, endpoint)
		case <-configCh:
			cfg := c.configurator.Config()
			interval = cfg.ClusterTransfer.Interval
			endpoint = cfg.ClusterTransfer.Endpoint
		}
	}
}

// requestDataAndUpdateSecret queries the provided endpoint. If there is any data
// in the response then check if a secret update is required, and if so, perform the update.
func (c *Controller) requestDataAndUpdateSecret(ctx context.Context, endpoint string) {
	klog.Infof("checking the availability of cluster transfer. Next check is in %s", c.configurator.Config().ClusterTransfer.Interval)
	data, err := c.requestClusterTransferWithExponentialBackoff(endpoint)
	if err != nil {
		msg := fmt.Sprintf("failed to pull cluster transfer: %v", err)
		httpErr, ok := err.(insightsclient.HttpError)
		// HTTP error received
		if ok {
			reason := strings.ReplaceAll(http.StatusText(httpErr.StatusCode), " ", "")
			c.updateStatus(false, msg, reason, &httpErr)
			return
		}
		// we are probably in disconnected environment
		klog.Errorf(msg)
		c.updateStatus(true, msg, disconnectedReason, nil)
		return
	}

	// there's no cluster transfer for the cluster - HTTP 204
	if len(data) == 0 {
		klog.Info("no available accepted cluster transfer")
		c.updateStatus(true, "no available cluster transfer", noClusterTransfer, nil)
		return
	}
	// deserialize the data from the OCM API
	ctList, err := unmarhallResponseData(data)
	if err != nil {
		msg := fmt.Sprintf("unable to deserialize the cluster transfer API response: %v", err)
		klog.Error(msg)
		c.updateStatus(false, msg, unexpectedData, nil)
		return
	}
	c.checkCTListAndOptionallyUpdatePS(ctx, ctList)
}

// checkCTListAndOptionallyUpdatePS checks the provided cluster transfer list length,
// If there is more than 1 accepted cluster transfer then log the message, update the controller status
// and do nothing. If there is only one accepted cluster transfer then
// check if the `pull-secret` needs to be updated. If the `pull-secret` needs to be updated then
// update it and update the controller status, otherwise just update the controller status.
func (c *Controller) checkCTListAndOptionallyUpdatePS(ctx context.Context, ctList *clusterTransferList) {
	if ctList.Total > 1 {
		msg := "there are more accepted cluster transfers. The pull-secret will not be updated!"
		klog.Infof(msg)
		c.updateStatus(true, msg, moreAcceptedClusterTransfers, nil)
		return
	}
	// this should not happen. This is just safe check
	if len(ctList.Transfers) != 1 {
		msg := "unexpected number of cluster transfers received from the API"
		klog.Infof(msg)
		c.updateStatus(true, msg, unexpectedData, nil)
		return
	}

	newPullSecret := []byte(ctList.Transfers[0].Secret)
	var statusMsg, reason string
	// check if the pull-secret needs to be updated
	updating, err := c.isUpdateRequired(ctx, newPullSecret)
	if err != nil {
		statusMsg = fmt.Sprintf("new pull-secret check failed: %v", err)
		klog.Errorf(statusMsg)
		c.updateStatus(false, statusMsg, dataCorrupted, nil)
		return
	}
	if updating {
		klog.Info("updating the pull-secret content")
		err = c.updatePullSecret(ctx, newPullSecret)
		if err != nil {
			statusMsg = fmt.Sprintf("failed to update pull-secret: %v", err)
			klog.Errorf(statusMsg)
			c.updateStatus(false, statusMsg, "UpdateFailed", nil)
			return
		}
		statusMsg = "pull-secret successfully updated"
		reason = AvailableReason
		klog.Info(statusMsg)
	} else {
		statusMsg = "no new data received"
		reason = "NoNewData"
		klog.Info(statusMsg)
	}
	c.updateStatus(true, statusMsg, reason, nil)
}

// isUpdateRequired checks if an update of the pull-secret is required or not.
func (c *Controller) isUpdateRequired(ctx context.Context, newData []byte) (bool, error) {
	pullSecret, err := c.getPullSecret(ctx)
	if err != nil {
		return false, err
	}
	c.pullSecret = pullSecret

	newPullSecret, err := unmarshallToPullSecretContent(newData)
	if err != nil {
		return false, err
	}
	originalPullSecret, err := unmarshallToPullSecretContent(pullSecret.Data[v1.DockerConfigJsonKey])
	if err != nil {
		return false, err
	}
	return !isUpdatedPullSecretContentSame(*originalPullSecret, *newPullSecret), nil
}

// updatePullSecret creates a JSON merge patch of existing and new pull-secret data.
// The result of the patch is used for pull-secret data update.
func (c *Controller) updatePullSecret(ctx context.Context, newData []byte) error {
	if c.pullSecret == nil {
		ps, err := c.getPullSecret(ctx)
		if err != nil {
			return err
		}
		c.pullSecret = ps
	}
	existingData := c.pullSecret.Data[v1.DockerConfigJsonKey]

	updatedData, err := jsonpatch.MergePatch(existingData, newData)
	if err != nil {
		return err
	}

	c.pullSecret.Data[v1.DockerConfigJsonKey] = updatedData
	_, err = c.coreClient.Secrets("openshift-config").Update(ctx, c.pullSecret, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

// requestClusterTransferWithExponentialBackoff queries a cluster transfer object
// from the OCM API with exponential backoff.
// It returns HttpError (see insightsclient.go) in case of any HTTP error response from the OCM API.
// The exponential backoff is applied only for HTTP errors >= 500.
func (c *Controller) requestClusterTransferWithExponentialBackoff(endpoint string) ([]byte, error) {
	bo := wait.Backoff{
		Duration: c.configurator.Config().ClusterTransfer.Interval / 24, // 30 min as the first waiting
		Factor:   2,
		Jitter:   0,
		Steps:    ocm.FailureCountThreshold,
		Cap:      c.configurator.Config().ClusterTransfer.Interval,
	}

	var data []byte
	err := wait.ExponentialBackoff(bo, func() (bool, error) {
		var err error
		data, err = c.client.RecvClusterTransfer(endpoint)
		if err == nil {
			return true, nil
		}
		// don't try again in case it's not an HTTP error - it could mean we're in disconnected env
		if !insightsclient.IsHttpError(err) {
			return true, err
		}
		httpErr := err.(insightsclient.HttpError)
		if httpErr.StatusCode >= http.StatusInternalServerError {
			// check the number of steps to prevent "timeout waiting for condition" error - we want to propagate the HTTP error below
			if bo.Steps > 1 {
				klog.Errorf("Got HTTP %v. Trying again in %s", httpErr.StatusCode, bo.Step())
				return false, nil
			}
		}
		return true, httpErr
	})
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (c *Controller) updateStatus(healthy bool, msg, reason string, httpErr *insightsclient.HttpError) {
	newSummary := controllerstatus.Summary{
		Operation: controllerstatus.Operation{
			Name: controllerstatus.PullingClusterTransfer.Name,
		},
		Healthy:            healthy,
		Reason:             reason,
		Message:            msg,
		LastTransitionTime: time.Now(),
	}
	if httpErr != nil {
		newSummary.Operation.HTTPStatusCode = httpErr.StatusCode
	}
	c.UpdateStatus(newSummary)
}

// getPullSecret gets pull-secret as *v1.Secret
func (c *Controller) getPullSecret(ctx context.Context) (*v1.Secret, error) {
	return c.coreClient.Secrets("openshift-config").Get(ctx, "pull-secret", metav1.GetOptions{})
}

// isUpdatedPullSecretContentSame checks if the updatedPS content is different
// (and thus update is required) to the originalPS content
func isUpdatedPullSecretContentSame(originalPS, updatedPS pullSecretContent) bool {
	for k, v := range updatedPS.Auths {
		existingAuth, ok := originalPS.Auths[k]
		if !ok {
			return false
		}
		if existingAuth != v {
			return false
		}
	}
	return true
}

// unmarshallToPullSecretContent unmarshals the data into "pullSecretContent" type.
// If the unmarshaling fails then an error is returned. Otherwise, the reference to
// the new pullSecretContent is returned.
func unmarshallToPullSecretContent(data []byte) (*pullSecretContent, error) {
	var psContent pullSecretContent
	err := json.Unmarshal(data, &psContent)
	if err != nil {
		return nil, err
	}
	return &psContent, nil
}

// unmarhallResponseData accepts slice of bytes and unmarshals it into `clusterTransferList` type
func unmarhallResponseData(data []byte) (*clusterTransferList, error) {
	var ctList clusterTransferList
	err := json.Unmarshal(data, &ctList)
	if err != nil {
		return nil, err
	}
	return &ctList, nil
}
