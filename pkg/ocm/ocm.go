package ocm

import (
	"context"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/openshift/insights-operator/pkg/config"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"
)

const (
	targetNamespaceName = "openshift-config-managed"
	secretName          = "etc-pki-entitlement"
)

// Controller holds all the required resources to be able to communicate with OCM API
type Controller struct {
	coreClient   corev1client.CoreV1Interface
	context      context.Context
	configurator Configurator
}

// Configurator represents the interface to retrieve the configuration for the gatherer
type Configurator interface {
	Config() *config.Controller
	ConfigChanged() (<-chan struct{}, func())
}

// New creates new instance
func New(ctx context.Context, coreClient corev1client.CoreV1Interface, configurator Configurator) *Controller {
	return &Controller{
		coreClient:   coreClient,
		context:      ctx,
		configurator: configurator,
	}
}

// Run periodically queries the OCM API and update corresponding secret accordingly
func (c *Controller) Run() {
	cfg := c.configurator.Config()
	endpoint := cfg.OcmEndpoint
	interval := cfg.OcmInterval
	configCh, cancel := c.configurator.ConfigChanged()
	defer cancel()

	c.requestDataAndCheckSecret(endpoint)
	for {
		select {
		case <-time.After(interval):
			c.requestDataAndCheckSecret(endpoint)
		case <-configCh:
			cfg := c.configurator.Config()
			interval = cfg.OcmInterval
			endpoint = cfg.OcmEndpoint
		}
	}
}

func (c *Controller) requestDataAndCheckSecret(endpoint string) {
	data, err := requestData(endpoint)
	if err != nil {
		klog.Errorf("errror requesting data from %s: %v", endpoint, err)
	}
	// check & update the secret here
	ok, err := c.checkSecret(data)
	if !ok {
		// TODO - change IO status in case of failure ?
		klog.Errorf("error when checking the %s secret: %v", secretName, err)
		return
	}
	klog.Infof("%s secret successfuly updated", secretName)

}

// checkSecret checks "simple-content-access" secret in the "openshift-config-managed" namespace.
// If the secret doesn't exist then it will create a new one.
// If the secret already exist then it will update the data.
func (c *Controller) checkSecret(data []byte) (bool, error) {
	scaSec, err := c.coreClient.Secrets(targetNamespaceName).Get(c.context, secretName, metav1.GetOptions{})

	//if the secret doesn't exist then create one
	if errors.IsNotFound(err) {
		_, err = c.createSecret(data)
		if err != nil {
			return false, err
		}
		return true, nil
	}
	if err != nil {
		return false, err
	}

	_, err = c.updateSecret(scaSec, data)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (o *Controller) createSecret(data []byte) (*v1.Secret, error) {
	newSCA := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: targetNamespaceName,
		},
		// TODO set the data correctly
		Data: map[string][]byte{
			v1.TLSCertKey:       data,
			v1.TLSPrivateKeyKey: data,
		},
		Type: v1.SecretTypeTLS,
	}
	cm, err := o.coreClient.Secrets(targetNamespaceName).Create(o.context, newSCA, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	return cm, nil
}

// updateSecret updates provided secret with given data
func (o *Controller) updateSecret(s *v1.Secret, data []byte) (*v1.Secret, error) {

	// TODO set the data correctly
	s.Data = map[string][]byte{
		v1.TLSCertKey:       data,
		v1.TLSPrivateKeyKey: data,
	}
	s, err := o.coreClient.Secrets(s.Namespace).Update(o.context, s, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	return s, nil
}

// TODO
// - no need to create new HTTP client every time
// - add authorization
// - add response status check
// - move this to insightsclient?
func requestData(endpoint string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	d, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	return d, nil
}
