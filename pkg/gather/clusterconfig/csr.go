package clusterconfig

import (
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/openshift/insights-operator/pkg/utils"
	"k8s.io/api/certificates/v1beta1"
	certificatesv1b1api "k8s.io/api/certificates/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
)

type CSRAnonymizer struct {
	*certificatesv1b1api.CertificateSigningRequest
}

func (a CSRAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	res, err := anonymizeCsr(a.CertificateSigningRequest)
	if err != nil {
		return nil, err
	}

	return json.Marshal(res)
}

func anonymizeCsrRequest(r *certificatesv1b1api.CertificateSigningRequest, c *CSRAnonymizedFeatures) {
	if r == nil || c == nil {
		return
	}
	c.Spec = &StateFeatures{}
	c.Spec.Username = r.Spec.Username
	c.Spec.Groups = r.Spec.Groups
	c.Spec.Usages = r.Spec.Usages

	// CSR in a PEM
	// parse only first PEM block
	block, _ := pem.Decode(r.Spec.Request)
	if block == nil {
		// unable to decode CSR: missing block
		return
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return
	}

	err = csr.CheckSignature()
	if err != nil {
		return
	}
	c.Spec.Request = &CsrFeatures{}
	c.Spec.Request.ValidSignature = err == nil
	c.Spec.Request.Subject = anonymizePkxName(csr.Subject)

	c.Spec.Request.SignatureAlgorithm = csr.SignatureAlgorithm.String()
	c.Spec.Request.PublicKeyAlgorithm = csr.PublicKeyAlgorithm.String()
	c.Spec.Request.DNSNames = utils.Map(csr.DNSNames, anonymizeURL)
	c.Spec.Request.EmailAddresses = utils.Map(csr.EmailAddresses, anonymizeURL)
	ipsl := []string{}
	for _, ip := range csr.IPAddresses {
		ipsl = append(ipsl, ip.String())
	}
	c.Spec.Request.IPAddresses = utils.Map(ipsl, anonymizeURL)
	urlsl := []string{}
	for _, u := range csr.URIs {
		urlsl = append(urlsl, u.String())
	}
	c.Spec.Request.URIs = utils.Map(urlsl, anonymizeURL)
}

func anonymizePkxName(s pkix.Name) (a pkix.Name) {
	its := func(n *pkix.Name) []interface{} {
		return []interface{}{
			&n.CommonName,
			&n.Locality,
			&n.Province,
			&n.StreetAddress,
			&n.PostalCode,
			&n.Country,
			&n.Organization,
			&n.OrganizationalUnit,
			&n.SerialNumber,
		}
	}

	src := its(&s)
	dst := its(&a)
	for i := range src {
		switch s := src[i].(type) {
		case *string:
			*(dst[i].(*string)) = anonymizeString(*s)
		case *[]string:
			*(dst[i].(*[]string)) = utils.Map(*s, anonymizeString)
		default:
			panic(fmt.Sprintf("unknown type %T", s))
		}
	}
	return
}

// returns true if certificate is valid
func anonymizeCsrCert(r *certificatesv1b1api.CertificateSigningRequest, c *CSRAnonymizedFeatures) {
	if r == nil || c == nil {
		return
	}
	c.Status = &StatusFeatures{}
	c.Status.Conditions = r.Status.Conditions
	// Certificate PEM
	// parse only first PEM block
	block, _ := pem.Decode(r.Status.Certificate)
	if block == nil {
		// unable to decode CSR: missing block
		return
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return
	}
	c.Status.Cert = &CertFeatures{}
	c.Status.Cert.Issuer = anonymizePkxName(cert.Issuer)
	c.Status.Cert.Subject = anonymizePkxName(cert.Subject)
	c.Status.Cert.NotBefore = cert.NotBefore.Format(time.RFC3339)
	c.Status.Cert.NotAfter = cert.NotAfter.Format(time.RFC3339)
}

func addMeta(r *certificatesv1b1api.CertificateSigningRequest, c *CSRAnonymizedFeatures) {
	if r == nil || c == nil {
		return
	}
	c.TypeMeta = r.TypeMeta
	c.ObjectMeta = r.ObjectMeta
}

func anonymizeCsr(r *certificatesv1b1api.CertificateSigningRequest) (*CSRAnonymizedFeatures, error) {
	c := &CSRAnonymizedFeatures{}
	addMeta(r, c)
	anonymizeCsrRequest(r, c)
	anonymizeCsrCert(r, c)
	return c, nil
}

type CSRAnonymizedFeatures struct {
	TypeMeta   metav1.TypeMeta
	ObjectMeta metav1.ObjectMeta
	Spec       *StateFeatures
	Status     *StatusFeatures
}

type StateFeatures struct {
	UID      string
	Username string
	Groups   []string
	Usages   []v1beta1.KeyUsage

	Request *CsrFeatures
}

type StatusFeatures struct {
	Conditions []v1beta1.CertificateSigningRequestCondition
	Cert       *CertFeatures
}

type CsrFeatures struct {
	ValidSignature     bool
	SignatureAlgorithm string
	PublicKeyAlgorithm string
	DNSNames           []string
	EmailAddresses     []string
	IPAddresses        []string
	URIs               []string
	Subject            pkix.Name
}

type CertFeatures struct {
	Verified  bool
	Issuer    pkix.Name
	Subject   pkix.Name
	NotBefore string
	NotAfter  string
}
