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
	targetNamespaceName        = "openshift-config-managed" //nolint: gosec
	secretName                 = "etc-pki-entitlement"      //nolint: gosec
	secretArchName             = "etc-pki-entitlement-%s"   //nolint: gosec
	entitlementAttrName        = "entitlement.pem"
	entitlementKeyAttrName     = "entitlement-key.pem"
	ControllerName             = "scaController"
	AvailableReason            = "Updated"
	SCAProcessingFailureReason = "FailedToProcessSCACerts"
)

// Mapping of architecture format used by SCA API to the format used by kubernetes
var archMapping = map[string]string{
	"x86":     "386",
	"x86_64":  "amd64",
	"ppc":     "ppc",
	"ppc64":   "ppc64",
	"ppc64le": "ppc64le",
	"s390":    "s390",
	"s390x":   "s390x",
	"ia64":    "ia64",
	"aarch64": "arm64",
}

// Controller holds all the required resources to be able to communicate with OCM API
type Controller struct {
	controllerstatus.StatusController
	coreClient   corev1client.CoreV1Interface
	configurator configobserver.Interface
	client       *insightsclient.Client
}

// Response structure is used to unmarshall the OCM SCA response.
type Response struct {
	Items []CertData `json:"items"`
	Kind  string     `json:"kind"`
	Total int        `json:"total"`
}

func (r *Response) getCertDataByName(archName string) *CertData {
	for _, archData := range r.Items {
		if archData.Metadata.Arch == archName {
			return &archData
		}
	}

	return nil
}

// CertData holds the SCA certificate
type CertData struct {
	ID       string       `json:"id"`
	OrgID    string       `json:"organization_id"`
	Key      string       `json:"key"`
	Cert     string       `json:"cert"`
	Metadata CertMetadata `json:"metadata"`
}

// ResonseMetadata structure is used to unmarshall the OCM SCA response metadata.
type CertMetadata struct {
	Arch string `json:"arch"`
}

// New creates new instance
func New(coreClient corev1client.CoreV1Interface, configurator configobserver.Interface,
	insightsClient *insightsclient.Client,
) *Controller {
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
				c.UpdateStatus(controllerstatus.Summary{
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

	clusterArchitectures, err := c.gatherArchitectures(ctx)
	if err != nil {
		klog.Errorf("Gathering nodes architectures failed: %s", err.Error())
		return
	}

	responses, err := c.requestSCAWithExpBackoff(ctx, endpoint, clusterArchitectures.NodeArchitectures)
	if err != nil {
		httpErr, ok := err.(insightsclient.HttpError)
		errMsg := fmt.Sprintf("Failed to pull SCA certs from %s: %v", endpoint, err)
		if ok {
			c.UpdateStatus(controllerstatus.Summary{
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
		c.UpdateStatus(controllerstatus.Summary{
			Operation:          controllerstatus.PullingSCACerts,
			Healthy:            true,
			Reason:             "NonHTTPError",
			Message:            errMsg,
			LastTransitionTime: time.Now(),
		})
		return
	}
	err = c.processResponses(ctx, *responses, clusterArchitectures.ControlPlaneArch)
	if err != nil {
		c.UpdateStatus(controllerstatus.Summary{
			Operation:          controllerstatus.PullingSCACerts,
			Message:            "Failed to process SCA certs: " + err.Error(),
			Healthy:            false,
			LastTransitionTime: time.Now(),
			Reason:             SCAProcessingFailureReason,
		})
		return
	}

	c.UpdateStatus(controllerstatus.Summary{
		Operation:          controllerstatus.PullingSCACerts,
		Message:            "SCA certs successfully updated",
		Healthy:            true,
		LastTransitionTime: time.Now(),
		Reason:             AvailableReason,
	})
}

func (c *Controller) processResponses(ctx context.Context, responses Response, controlPlaneArch string) error {
	if responses.Total == 1 {
		// If there is only one architecture then we will use the secret name "etc-pki-entitlement"
		// without the arch suffix to keep the backward compatibility
		return c.checkSecret(ctx, &responses.Items[0], secretName)
	}

	controlPlaneCertData := responses.getCertDataByName(controlPlaneArch)
	if controlPlaneCertData != nil {
		// Create or update the default "etc-pki-entitlement" secret with the control plane node data
		if err := c.checkSecret(ctx, controlPlaneCertData, secretName); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("certificates for node architecture not found, default secret is not created nor updated")
	}

	// Create architecture specific secrets with sca certificates
	for _, response := range responses.Items {
		if err := c.checkSecret(ctx, &response, fmt.Sprintf(secretArchName, archMapping[response.Metadata.Arch])); err != nil {
			return err
		}
	}

	return nil
}

// checkSecret checks "etc-pki-entitlement" or "etc-pki-entitlement-arch"
// secret in the "openshift-config-managed" namespace.
// If the secret doesn't exist then it will create a new one.
// If the secret already exist then it will update the data.
func (c *Controller) checkSecret(ctx context.Context, ocmData *CertData, secretArchName string) error {
	scaSec, err := c.coreClient.Secrets(targetNamespaceName).Get(ctx, secretArchName, metav1.GetOptions{})

	// if the secret doesn't exist then create one
	if errors.IsNotFound(err) {
		_, err = c.createSecret(ctx, ocmData, secretArchName)
		if err != nil {
			klog.Errorf("Error when creating the %s secret: %v", secretArchName, err)
			return err
		}
		return nil
	}
	if err != nil {
		klog.Errorf("Error getting the %s secret: %v", secretArchName, err)
		return err
	}

	_, err = c.updateSecret(ctx, scaSec, ocmData)
	if err != nil {
		klog.Errorf("Error when updating the %s secret: %v", secretName, err)
		return err
	}
	return nil
}

func (c *Controller) createSecret(ctx context.Context, ocmData *CertData, secretArchName string) (*v1.Secret, error) {
	newSCA := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretArchName,
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

	klog.Infof("%s secret successfully created", newSCA.Name)

	return cm, nil
}

// updateSecret updates provided secret with given data
func (c *Controller) updateSecret(ctx context.Context, s *v1.Secret, ocmData *CertData) (*v1.Secret, error) {
	s.Data = map[string][]byte{
		entitlementAttrName:    []byte(ocmData.Cert),
		entitlementKeyAttrName: []byte(ocmData.Key),
	}

	s, err := c.coreClient.Secrets(s.Namespace).Update(ctx, s, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}

	klog.Infof("%s secret successfully updated", s.Name)

	return s, nil
}

// requestSCAWithExpBackoff queries OCM API with exponential backoff.
// Returns HttpError (see insightsclient.go) in case of any HTTP error response from OCM API.
// The exponential backoff is applied only for HTTP errors >= 500.
func (c *Controller) requestSCAWithExpBackoff(
	ctx context.Context, endpoint string, nodeArchitectures map[string]struct{},
) (*Response, error) {
	bo := wait.Backoff{
		Duration: c.configurator.Config().SCA.Interval / 32, // 15 min by default
		Factor:   2,
		Jitter:   0,
		Steps:    ocm.FailureCountThreshold,
		Cap:      c.configurator.Config().SCA.Interval,
	}

	var err error
	var data []byte
	err = wait.ExponentialBackoff(bo, func() (bool, error) {
		data, err = c.client.RecvSCACerts(ctx, endpoint, nodeArchitectures)
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

	var response Response
	err = json.Unmarshal(data, &response)
	if err != nil {
		klog.Errorf("Unable to decode response: %v", err)
		return nil, err
	}

	return &response, nil
}
