package ocm

import (
	"context"
	"encoding/json"
	"time"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"
)

const (
	targetNamespaceName = "openshift-config-managed"
	secretName          = "etc-pki-entitlement" //nolint: gosec
)

// Controller holds all the required resources to be able to communicate with OCM API
type Controller struct {
	coreClient   corev1client.CoreV1Interface
	ctx          context.Context
	configurator Configurator
	client       *insightsclient.Client
}

// Configurator represents the interface to retrieve the configuration for the gatherer
type Configurator interface {
	Config() *config.Controller
	ConfigChanged() (<-chan struct{}, func())
}

// ScaResponse structure is used to unmarshall the OCM response. It holds the SCA certificate
type ScaResponse struct {
	ID    string `json:"id"`
	OrgID string `json:"organization_id"`
	Key   string `json:"key"`
	Cert  string `json:"cert"`
}

// New creates new instance
func New(ctx context.Context, coreClient corev1client.CoreV1Interface, configurator Configurator,
	insightsClient *insightsclient.Client) *Controller {
	return &Controller{
		coreClient:   coreClient,
		ctx:          ctx,
		configurator: configurator,
		client:       insightsClient,
	}
}

// Run periodically queries the OCM API and update corresponding secret accordingly
func (c *Controller) Run() {
	cfg := c.configurator.Config()
	endpoint := cfg.OCMConfig.Endpoint
	interval := cfg.OCMConfig.Interval
	disabled := cfg.OCMConfig.Disabled
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
			}
		case <-configCh:
			cfg := c.configurator.Config()
			interval = cfg.OCMConfig.Interval
			endpoint = cfg.OCMConfig.Endpoint
			disabled = cfg.OCMConfig.Disabled
		}
	}
}

func (c *Controller) requestDataAndCheckSecret(endpoint string) {
	data, err := c.client.RecvSCACerts(c.ctx, endpoint)
	if err != nil {
		klog.Errorf("Failed to retrieve data: %v", err)
		return
	}
	var ocmRes ScaResponse
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
}

// checkSecret checks "etc-pki-entitlement" secret in the "openshift-config-managed" namespace.
// If the secret doesn't exist then it will create a new one.
// If the secret already exist then it will update the data.
func (c *Controller) checkSecret(ocmData *ScaResponse) error {
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

func (c *Controller) createSecret(ocmData *ScaResponse) (*v1.Secret, error) {
	newSCA := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: targetNamespaceName,
		},
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte(ocmData.Cert),
			v1.TLSPrivateKeyKey: []byte(ocmData.Key),
		},
		Type: v1.SecretTypeTLS,
	}
	cm, err := c.coreClient.Secrets(targetNamespaceName).Create(c.ctx, newSCA, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	return cm, nil
}

// updateSecret updates provided secret with given data
func (c *Controller) updateSecret(s *v1.Secret, ocmData *ScaResponse) (*v1.Secret, error) {
	s.Data = map[string][]byte{
		v1.TLSCertKey:       []byte(ocmData.Cert),
		v1.TLSPrivateKeyKey: []byte(ocmData.Key),
	}
	s, err := c.coreClient.Secrets(s.Namespace).Update(c.ctx, s, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	return s, nil
}
