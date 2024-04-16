package clusterconfig

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	operatorclient "github.com/openshift/client-go/operator/clientset/versioned"

	"k8s.io/klog/v2"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

const ingressCertificatesLimits = int64(64)

var ingressNamespaces = []string{
	"openshift-ingress-operator",
	"openshift-ingress",
}

type CertificateInfo struct {
	Name        string           `json:"name"`
	Namespace   string           `json:"namespace"`
	NotBefore   metav1.Time      `json:"not_before"`
	NotAfter    metav1.Time      `json:"not_after"`
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

	var certificates []*CertificateInfo
	var errs []error

	// Step 1: Collect router-ca and router-certs-default
	routerCACert, routerCACertErr := getCertificateInfoFromSecret(ctx, coreClient, "openshift-ingress-operator", "router-ca")
	if routerCACertErr != nil {
		errs = append(errs, routerCACertErr)
	} else {
		certificates = append(certificates, routerCACert)
	}

	routerCertsDefaultCert, routerCertsDefaultCertErr := getCertificateInfoFromSecret(ctx, coreClient, "openshift-ingress", "router-certs-default")
	if routerCertsDefaultCertErr != nil {
		errs = append(errs, routerCertsDefaultCertErr)
	} else {
		certificates = append(certificates, routerCertsDefaultCert)
	}

	// Step 2: List all Ingress Controllers
	for _, namespace := range ingressNamespaces {
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
			if controller.Spec.DefaultCertificate != nil {
				certName := controller.Spec.DefaultCertificate.Name
				certInfo, certErr := getCertificateInfoFromSecret(ctx, coreClient, namespace, certName)
				if certErr != nil {
					errs = append(errs, certErr)
					continue
				}

				// Step 4: Add certificate info to the certificates list
				found := false
				for _, cert := range certificates {
					if cert.Name == certInfo.Name {
						// Certificate already exists, add the controller to its list
						cert.Controllers = append(cert.Controllers, ControllerInfo{Name: controller.Name, Namespace: controller.Namespace})
						found = true
						break
					}
				}

				if !found {
					// Certificate not found, create a new entry
					certificates = append(certificates, certInfo)
				}
			}
		}
	}

	var records []record.Record
	if len(certificates) > 0 {
		// Step 5: Generate the certificates record
		records = append(records, record.Record{
			Name: "config/ingress/certificates",
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
		Name:      secretName,
		Namespace: namespace,
		NotBefore: metav1.NewTime(cert.NotBefore),
		NotAfter:  metav1.NewTime(cert.NotAfter),
		Controllers: []ControllerInfo{
			{Name: "router", Namespace: namespace},
		},
	}, nil
}
