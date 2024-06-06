package clusterconfig

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	operatorclient "github.com/openshift/client-go/operator/clientset/versioned"

	"k8s.io/klog/v2"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

const ingressCertificatesLimits = 64

type CertificateInfo struct {
	Name        string           `json:"name"`
	Namespace   string           `json:"namespace"`
	NotBefore   time.Time        `json:"not_before"`
	NotAfter    time.Time        `json:"not_after"`
	Controllers []ControllerInfo `json:"controllers"`
}

type ControllerInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (g *Gatherer) GatherClusterIngressCertificates(ctx context.Context) ([]record.Record, []error) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	operatorClient, err := operatorclient.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherClusterIngressCertificates(ctx, gatherKubeClient.CoreV1(), operatorClient)
}

func gatherClusterIngressCertificates(
	ctx context.Context,
	coreClient corev1client.CoreV1Interface,
	operatorClient operatorclient.Interface) ([]record.Record, []error) {

	const Filename = "aggregated/ingress_controllers_certs"

	var ingressAllowedNS = [2]string{"openshift-ingress-operator", "openshift-ingress"}

	var certificates []*CertificateInfo
	var errs []error

	// Step 1: Collect router-ca and router-certs-default
	rCAinfo, err := getRouterCACertInfo(ctx, coreClient)
	if err != nil {
		errs = append(errs, err)
	}
	if rCAinfo != nil {
		certificates = append(certificates, rCAinfo)
	}

	rCDinfo, err := getRouterCertsDefaultCertInfo(ctx, coreClient)
	if err != nil {
		errs = append(errs, err)
	}
	if rCDinfo != nil {
		certificates = append(certificates, rCDinfo)
	}

	// Step 2: List all Ingress Controllers
	for _, namespace := range ingressAllowedNS {
		controllers, err := operatorClient.OperatorV1().IngressControllers(namespace).List(ctx, metav1.ListOptions{})
		if errors.IsNotFound(err) {
			klog.V(2).Infof("Ingress Controllers not found in '%s' namespace", namespace)
			continue
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}

		// Step 3: Filter Ingress Controllers with spec.defaultCertificate and get certificate info
		for _, controller := range controllers.Items {

			// Step 4: Check the certificate limits
			if len(certificates) >= ingressCertificatesLimits {
				klog.V(2).Infof("Reached the limit of ingress certificates (%d), skipping additional certificates", ingressCertificatesLimits)
				break
			}

			if controller.Spec.DefaultCertificate != nil {
				certName := controller.Spec.DefaultCertificate.Name // TODO - rename to secretname
				certInfo, certErr := getCertificateInfoFromSecret(ctx, coreClient, namespace, certName)
				if certErr != nil {
					errs = append(errs, certErr)
					continue
				}

				// Step 5: Add certificate info to the certificates list
				found := false
				for _, cert := range certificates {
					// I need to compare both, since a certificate seems to be able to live outside the controller's namespace
					if cert.Name == certInfo.Name && cert.Namespace == certInfo.Namespace {
						// Certificate already exists, add the controller to its list
						cert.Controllers = append(cert.Controllers, ControllerInfo{Name: controller.Name, Namespace: controller.Namespace})
						found = true
						break
					}
				}

				if !found {
					// Certificate not found, create a new entry
					certInfo.Controllers = append(certInfo.Controllers, ControllerInfo{Name: controller.Name, Namespace: controller.Namespace})
					certificates = append(certificates, certInfo)
				}
			}
		}
	}

	if len(errs) > 0 {
		return nil, errs
	}

	var records []record.Record
	if len(certificates) > 0 {
		// Step 6: Generate the certificates record
		records = append(records, record.Record{
			Name: Filename,
			Item: record.JSONMarshaller{Object: certificates},
		})
	}

	return records, nil
}

func getCertificateInfoFromSecret(ctx context.Context, coreClient corev1client.CoreV1Interface, namespace, secretName string) (*CertificateInfo, error) {
	secret, err := coreClient.Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret '%s' in namespace '%s': %v", secretName, namespace, err)
	}

	crtData, found := secret.Data["tls.crt"]
	if !found {
		return nil, fmt.Errorf("'tls.crt' not found in secret '%s' in namespace '%s'", secretName, namespace)
	}

	block, _ := pem.Decode(crtData)
	if block == nil {
		return nil, fmt.Errorf("unable to decode certificate (x509) from secret '%s' in namespace '%s'", secretName, namespace)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate from secret '%s' in namespace '%s': %v", secretName, namespace, err)
	}

	return &CertificateInfo{
		Name:        secretName,
		Namespace:   namespace,
		NotBefore:   cert.NotBefore,
		NotAfter:    cert.NotAfter,
		Controllers: []ControllerInfo{},
	}, nil
}

func getRouterCACertInfo(ctx context.Context, coreClient corev1client.CoreV1Interface) (*CertificateInfo, error) {
	const (
		routerCASecret    string = "router-ca"
		routerCANamespace string = "openshift-ingress-operator"
	)
	certInfo, err := getCertificateInfoFromSecret(ctx, coreClient, routerCANamespace, routerCASecret)
	if err != nil {
		return nil, err
	}

	return certInfo, nil
}

func getRouterCertsDefaultCertInfo(ctx context.Context, coreClient corev1client.CoreV1Interface) (*CertificateInfo, error) {
	const (
		routerCertsSecret    string = "router-certs-default"
		routerCertsNamespace string = "openshift-ingress"
	)
	certInfo, err := getCertificateInfoFromSecret(ctx, coreClient, routerCertsNamespace, routerCertsSecret)
	if err != nil {
		return nil, err
	}

	return certInfo, nil
}
