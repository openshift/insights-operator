package sca

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/controllerstatus"
	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
	"github.com/openshift/insights-operator/pkg/ocm"
)

const (
	targetNamespaceName    = "openshift-config-managed"
	secretName             = "etc-pki-entitlement" //nolint: gosec
	entitlementAttrName    = "entitlement.pem"
	entitlementKeyAttrName = "entitlement-key.pem"
	ControllerName         = "scaController"
	AvailableReason        = "Updated"
)

// Controller holds all the required resources to be able to communicate with OCM API
type Controller struct {
	controllerstatus.StatusController
	coreClient   corev1client.CoreV1Interface
	ctx          context.Context
	configurator configobserver.Configurator
	client       *insightsclient.Client
}

// Response structure is used to unmarshall the OCM SCA response. It holds the SCA certificate
type Response struct {
	ID    string `json:"id"`
	OrgID string `json:"organization_id"`
	Key   string `json:"key"`
	Cert  string `json:"cert"`
}

// New creates new instance
func New(ctx context.Context, coreClient corev1client.CoreV1Interface, configurator configobserver.Configurator,
	insightsClient *insightsclient.Client) *Controller {
	return &Controller{
		StatusController: controllerstatus.New(ControllerName),
		coreClient:       coreClient,
		ctx:              ctx,
		configurator:     configurator,
		client:           insightsClient,
	}
}

// Run periodically queries the OCM API and update corresponding secret accordingly
func (c *Controller) Run() {
	cfg := c.configurator.Config()
	endpoint := cfg.OCMConfig.SCAEndpoint
	interval := cfg.OCMConfig.SCAInterval
	disabled := cfg.OCMConfig.SCADisabled
	configCh, cancel := c.configurator.ConfigChanged()
	defer cancel()
	if !disabled {
		c.requestDataAndCheckSecret(endpoint)
	}
	for {
		select {
		case <-time.After(interval):
			if !disabled {
				c.requestDataAndCheckSecret(endpoint)
			} else {
				msg := "Pulling of the SCA certs from the OCM API is disabled"
				klog.Warning(msg)
				c.StatusController.UpdateStatus(controllerstatus.Summary{
					Operation:          controllerstatus.PullingSCACerts,
					Healthy:            true,
					Message:            msg,
					Reason:             "Disabled",
					LastTransitionTime: time.Now(),
				})
			}
		case <-configCh:
			cfg := c.configurator.Config()
			interval = cfg.OCMConfig.SCAInterval
			endpoint = cfg.OCMConfig.SCAEndpoint
			disabled = cfg.OCMConfig.SCADisabled
		}
	}
}

func (c *Controller) requestDataAndCheckSecret(endpoint string) {
	klog.Infof("Pulling SCA certificates from %s. Next check is in %s", c.configurator.Config().OCMConfig.SCAEndpoint,
		c.configurator.Config().OCMConfig.SCAInterval)
	data, err := c.requestSCAWithExpBackoff(endpoint)
	if err != nil {
		httpErr, ok := err.(insightsclient.HttpError)
		errMsg := fmt.Sprintf("Failed to pull SCA certs from %s: %v", endpoint, err)
		if ok {
			c.StatusController.UpdateStatus(controllerstatus.Summary{
				Operation: controllerstatus.Operation{
					Name:           controllerstatus.PullingSCACerts.Name,
					HTTPStatusCode: httpErr.StatusCode,
				},
				Reason:             strings.ReplaceAll(http.StatusText(httpErr.StatusCode), " ", ""),
				Message:            errMsg,
				LastTransitionTime: time.Now(),
			})
			return
		}
		klog.Warningf(errMsg)
		c.StatusController.UpdateStatus(controllerstatus.Summary{
			Operation:          controllerstatus.PullingSCACerts,
			Healthy:            true,
			Reason:             "NonHTTPError",
			Message:            errMsg,
			LastTransitionTime: time.Now(),
		})
		return
	}

	var ocmRes Response
	err = json.Unmarshal(data, &ocmRes)
	if err != nil {
		klog.Errorf("Unable to decode response: %v", err)
		return
	}

	// check & update the secret here
	err = c.checkSecret(&ocmRes)
	if err != nil {
		klog.Errorf("Error when checking the %s secret: %v", secretName, err)
		return
	}
	klog.Infof("%s secret successfully updated", secretName)
	c.StatusController.UpdateStatus(controllerstatus.Summary{
		Operation:          controllerstatus.PullingSCACerts,
		Message:            fmt.Sprintf("SCA certs successfully updated in the %s secret", secretName),
		Healthy:            true,
		LastTransitionTime: time.Now(),
		Reason:             AvailableReason,
	})
}

// checkSecret checks "etc-pki-entitlement" secret in the "openshift-config-managed" namespace.
// If the secret doesn't exist then it will create a new one.
// If the secret already exist then it will update the data.
func (c *Controller) checkSecret(ocmData *Response) error {
	scaSec, err := c.coreClient.Secrets(targetNamespaceName).Get(c.ctx, secretName, metav1.GetOptions{})

	// if the secret doesn't exist then create one
	if errors.IsNotFound(err) {
		_, err = c.createSecret(ocmData)
		if err != nil {
			return err
		}
		return nil
	}
	if err != nil {
		return err
	}

	_, err = c.updateSecret(scaSec, ocmData)
	if err != nil {
		return err
	}
	return nil
}

func (c *Controller) createSecret(ocmData *Response) (*v1.Secret, error) {
	newSCA := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: targetNamespaceName,
		},
		Data: map[string][]byte{
			entitlementAttrName:    []byte(ocmData.Cert),
			entitlementKeyAttrName: []byte(ocmData.Key),
		},
		Type: v1.SecretTypeOpaque,
	}
	cm, err := c.coreClient.Secrets(targetNamespaceName).Create(c.ctx, newSCA, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	return cm, nil
}

// updateSecret updates provided secret with given data
func (c *Controller) updateSecret(s *v1.Secret, ocmData *Response) (*v1.Secret, error) {
	s.Data = map[string][]byte{
		entitlementAttrName:    []byte(ocmData.Cert),
		entitlementKeyAttrName: []byte(ocmData.Key),
	}
	s, err := c.coreClient.Secrets(s.Namespace).Update(c.ctx, s, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	return s, nil
}

// requestSCAWithExpBackoff queries OCM API with exponential backoff.
// Returns HttpError (see insightsclient.go) in case of any HTTP error response from OCM API.
// The exponential backoff is applied only for HTTP errors >= 500.
func (c *Controller) requestSCAWithExpBackoff(endpoint string) ([]byte, error) {
	bo := wait.Backoff{
		Duration: c.configurator.Config().OCMConfig.SCAInterval / 32, // 15 min by default
		Factor:   2,
		Jitter:   0,
		Steps:    ocm.OCMAPIFailureCountThreshold,
		Cap:      c.configurator.Config().OCMConfig.SCAInterval,
	}
	var data []byte
	err := wait.ExponentialBackoff(bo, func() (bool, error) {
		var err error
		data, err = c.client.RecvSCACerts(c.ctx, endpoint)
		if err != nil {
			// don't try again in case it's not an HTTP error - it could mean we're in disconnected env
			if !insightsclient.IsHttpError(err) {
				return true, err
			}
			httpErr := err.(insightsclient.HttpError)
			// try again only in case of 500 or higher
			if httpErr.StatusCode >= http.StatusInternalServerError {
				// check the number of steps to prevent "timeout waiting for condition" error - we want to propagate the HTTP error
				if bo.Steps > 1 {
					klog.Errorf("%v. Trying again in %s", httpErr, bo.Step())
					return false, nil
				}
			}
			return true, httpErr
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return data, nil
}
