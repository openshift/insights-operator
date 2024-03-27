package clusterconfig

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"k8s.io/klog/v2"

	v1 "k8s.io/api/core/v1"

	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

// This map defines the namespace and the certificates that we are looking for
var ingressCertsMap = map[string][]string{
	"openshift-ingress-operator": {"router-ca"},
	"openshift-ingress":          {"router-certs-default"},
}

type IngressCertificateInfo struct {
	Name      string    `json:"name"`
	NotBefore time.Time `json:"not_before"`
	NotAfter  time.Time `json:"not_after"`
}

type IngressControllerInfo struct {
	Name                         string                   `json:"name"`
	OperatorGeneratedCertificate []IngressCertificateInfo `json:"operator_generated_certificate"`
	CustomCertificates           []IngressCertificateInfo `json:"custom_certificates"`
}

func (g *Gatherer) GatherClusterIngressCertificates(ctx context.Context) ([]record.Record, []error) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	configClient, err := configv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherClusterIngressCertificates(ctx, gatherKubeClient.CoreV1(), configClient)
}

func gatherClusterIngressCertificates(ctx context.Context, coreClient corev1client.CoreV1Interface, configClient configv1client.ConfigV1Interface) ([]record.Record, []error) {
	var records []record.Record
	var errors []error

	for namespace, certs := range ingressCertsMap {
		ingressCerts, errs := getIngressCertificates(ctx, coreClient, configClient, namespace, certs)
		if len(errs) > 0 {
			errors = append(errors, errs...)
			continue
		}

		if len(ingressCerts) > 0 {
			records = append(records, record.Record{
				Name: fmt.Sprintf("config/ingress/%s/ingress_certificates.json", namespace),
				Item: record.JSONMarshaller{Object: ingressCerts},
			})
		}
	}

	return records, nil
}

func getIngressCertificates(
	ctx context.Context,
	coreClient corev1client.CoreV1Interface,
	openshiftClient configv1client.ConfigV1Interface,
	namespace string,
	certs []string) ([]IngressControllerInfo, []error) {

	var controllers []IngressControllerInfo
	var errors []error

	ingressControllers, err := openshiftClient.Ingresses().List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.V(2).Infof("failed to list IngressControllers: %v", err)
		return nil, []error{err}
	}

	for _, controller := range ingressControllers.Items {
		controllerName := controller.Name
		controllerCertificates, errs := getControllerCertificates(ctx, coreClient, namespace, certs)
		if len(errs) > 0 {
			errors = append(errors, errs...)
			continue
		}

		controllers = append(controllers, IngressControllerInfo{
			Name:                         controllerName,
			OperatorGeneratedCertificate: controllerCertificates,
			CustomCertificates:           make([]IngressCertificateInfo, 0),
		})
	}

	return controllers, errors
}

func getControllerCertificates(
	ctx context.Context,
	coreClient corev1client.CoreV1Interface,
	namespace string,
	certs []string) ([]IngressCertificateInfo, []error) {

	var certInfos []IngressCertificateInfo
	var errors []error

	for _, secretName := range certs {
		secret, err := coreClient.Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			klog.V(2).Infof("failed to fetch secret: %v", err)
			errors = append(errors, err)
			continue
		}

		certInfo, err := certificateInfoFromSecret(secret)
		if err != nil {
			klog.V(2).Infof("failed to parse the ingress certificate: %v", err)
			errors = append(errors, err)
			continue
		}

		certInfos = append(certInfos, *certInfo)
	}

	return certInfos, errors
}

func certificateInfoFromSecret(secret *v1.Secret) (*IngressCertificateInfo, error) {
	crtData, found := secret.Data["tls.crt"]
	if !found {
		return nil, fmt.Errorf("'tls.crt' not found")
	}

	block, _ := pem.Decode(crtData)
	if block == nil {
		return nil, fmt.Errorf("unable to decode certificate (x509)")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	return &IngressCertificateInfo{
		Name:      secret.Name,
		NotBefore: cert.NotBefore,
		NotAfter:  cert.NotAfter,
	}, nil
}
