package sca

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
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
	targetNamespaceName    = "openshift-config-managed" //nolint: gosec
	secretName             = "etc-pki-entitlement"      //nolint: gosec
	entitlementAttrName    = "entitlement.pem"
	entitlementKeyAttrName = "entitlement-key.pem"
	ControllerName         = "scaController"
	AvailableReason        = "Updated"
)

// Controller holds all the required resources to be able to communicate with OCM API
type Controller struct {
	controllerstatus.StatusController
	coreClient   corev1client.CoreV1Interface
	configurator configobserver.Interface
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
func New(coreClient corev1client.CoreV1Interface, configurator configobserver.Interface,
	insightsClient *insightsclient.Client) *Controller {
	return &Controller{
		StatusController: controllerstatus.New(ControllerName),
		coreClient:       coreClient,
		configurator:     configurator,
		client:           insightsClient,
	}
}

// Run periodically queries the OCM API and update corresponding secret accordingly
func (c *Controller) Run(ctx context.Context) {
	cfg := c.configurator.Config()
	endpoint := cfg.SCA.Endpoint
	interval := cfg.SCA.Interval
	disabled := cfg.SCA.Disabled
	configCh, cancel := c.configurator.ConfigChanged()
	defer cancel()
	if !disabled {
		c.requestDataAndCheckSecret(ctx, endpoint)
	}
	for {
		select {
		case <-time.After(interval):
			if !disabled {
				c.requestDataAndCheckSecret(ctx, endpoint)
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
			interval = cfg.SCA.Interval
			endpoint = cfg.SCA.Endpoint
			disabled = cfg.SCA.Disabled
		}
	}
}

func (c *Controller) requestDataAndCheckSecret(ctx context.Context, endpoint string) {
	klog.Infof("Pulling SCA certificates from %s. Next check is in %s", c.configurator.Config().SCA.Endpoint,
		c.configurator.Config().SCA.Interval)

	architectures, err := c.gatherArchitectures(ctx)
	if err != nil {
		klog.Warningf("Gathering nodes architectures failed: %s", err.Error())
	}
	responses, err := c.requestSCAWithExpBackoff(ctx, endpoint, architectures)
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
		klog.Warning(errMsg)
		c.StatusController.UpdateStatus(controllerstatus.Summary{
			Operation:          controllerstatus.PullingSCACerts,
			Healthy:            true,
			Reason:             "NonHTTPError",
			Message:            errMsg,
			LastTransitionTime: time.Now(),
		})
		return
	}

	for idx := range responses {
		var ocmRes Response
		err = json.Unmarshal(responses[idx], &ocmRes)
		if err != nil {
			klog.Errorf("Unable to decode response: %v", err)
			return
		}
		// check & update the secret here
		err = c.checkSecret(ctx, &ocmRes)
		if err != nil {
			klog.Errorf("Error when checking the %s secret: %v", secretName, err)
			return
		}
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
func (c *Controller) checkSecret(ctx context.Context, ocmData *Response) error {
	scaSec, err := c.coreClient.Secrets(targetNamespaceName).Get(ctx, secretName, metav1.GetOptions{})

	// if the secret doesn't exist then create one
	if errors.IsNotFound(err) {
		_, err = c.createSecret(ctx, ocmData)
		if err != nil {
			return err
		}
		return nil
	}
	if err != nil {
		return err
	}

	_, err = c.updateSecret(ctx, scaSec, ocmData)
	if err != nil {
		return err
	}
	return nil
}

func (c *Controller) createSecret(ctx context.Context, ocmData *Response) (*v1.Secret, error) {
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
	cm, err := c.coreClient.Secrets(targetNamespaceName).Create(ctx, newSCA, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	return cm, nil
}

// updateSecret updates provided secret with given data
func (c *Controller) updateSecret(ctx context.Context, s *v1.Secret, ocmData *Response) (*v1.Secret, error) {
	s.Data = map[string][]byte{
		entitlementAttrName:    []byte(ocmData.Cert),
		entitlementKeyAttrName: []byte(ocmData.Key),
	}
	s, err := c.coreClient.Secrets(s.Namespace).Update(ctx, s, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	return s, nil
}

// getArch check the value of GOARCH and return a valid representation for
// OCM certificates API
func getArch() string {
	validArchs := map[string]string{
		"amd64": "x86_64",
		"i386":  "x86",
		"386":   "x86",
		"arm64": "aarch64",
	}

	if translation, ok := validArchs[runtime.GOARCH]; ok {
		return translation
	}
	return runtime.GOARCH
}

// requestSCAWithExpBackoff queries OCM API with exponential backoff.
// Returns HttpError (see insightsclient.go) in case of any HTTP error response from OCM API.
// The exponential backoff is applied only for HTTP errors >= 500.
func (c *Controller) requestSCAWithExpBackoff(ctx context.Context, endpoint string, architectures map[string]struct{}) ([][]byte, error) {
	bo := wait.Backoff{
		Duration: c.configurator.Config().SCA.Interval / 32, // 15 min by default
		Factor:   2,
		Jitter:   0,
		Steps:    ocm.FailureCountThreshold,
		Cap:      c.configurator.Config().SCA.Interval,
	}

	klog.Infof("Nodes architectures: %s", architectures)
	var responses [][]byte
	responses = make([][]byte, len(architectures))
	err := wait.ExponentialBackoff(bo, func() (bool, error) {
		for arch := range architectures {
			data, err := c.client.RecvSCACerts(ctx, endpoint, arch)
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

			responses = append(responses, data)
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return responses, nil
}
