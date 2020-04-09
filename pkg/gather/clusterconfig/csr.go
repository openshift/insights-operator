package clusterconfig

import (
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"time"

	"k8s.io/api/certificates/v1beta1"
	certificatesv1b1api "k8s.io/api/certificates/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/klog"
)

type CSRAnonymizer struct {
	*CSRAnonymizedFeatures
}

func (a CSRAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	// json.Marshal can handle nil well
	return json.Marshal(a.CSRAnonymizedFeatures)
}

type CSRs struct {
	Requests   []v1beta1.CertificateSigningRequest
	Anonymized []CSRAnonymizer
}

func FromCSRs(requests *v1beta1.CertificateSigningRequestList) *CSRs {
	return &CSRs{Requests: requests.Items}
}

func (c *CSRs) Anonymize() *CSRs {
	res := &CSRs{}
	for _, r := range c.Requests {
		af := anonymizeCSR(&r)
		res.Anonymized = append(res.Anonymized, CSRAnonymizer{af})
	}
	return res
}

func (c *CSRs) Filter(f FilterFeatures) *CSRs {
	res := &CSRs{}
	for _, r := range c.Anonymized {
		if f(r.CSRAnonymizedFeatures) {
			res.Anonymized = append(res.Anonymized, r)
		}
	}
	return res
}

func (c *CSRs) Select() ([]CSRAnonymizer, error) {
	return c.Anonymized, nil
}

type FilterFeatures func(c *CSRAnonymizedFeatures, opt ...FilterOptFunc) bool

type FilterOpt struct {
	time time.Time
}

type FilterOptFunc = func(o *FilterOpt)

func WithTime(t time.Time) FilterOptFunc {
	return func(o *FilterOpt) {
		o.time = t
	}
}

func IncludeCSR(c *CSRAnonymizedFeatures, opts ...FilterOptFunc) bool {
	opt := &FilterOpt{time: time.Now()}
	for _, o := range opts {
		o(opt)
	}
	// If we have a Cert for this CSR already issued
	if c.Status != nil && c.Status.Cert != nil {
		// CSR was valid and certificate exists
		if !c.Status.Cert.Verified {
			return true
		}
		if t, e := time.Parse(time.RFC3339, c.Status.Cert.NotBefore); e == nil && opt.time.Before(t) {
			// Now < Certificate NotBefore, certificate is probably not valid
			return true
		}
		if t, e := time.Parse(time.RFC3339, c.Status.Cert.NotAfter); e == nil && opt.time.After(t) {
			// Now > Certificate NotAfter, certificate is probably not valid
			return true
		}
		// Otherwise it may be valid valid and we dont collect it
		return false
	}
	// We dont know how CSR is going to be evaluated, collect it
	return true
}

func anonymizeCSRRequest(r *certificatesv1b1api.CertificateSigningRequest, c *CSRAnonymizedFeatures) {
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
		klog.V(2).Infof("Unable to decode PEM Request block for CSR %s in namespace %s. Missing block.", r.Name, r.Namespace)
		return
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		klog.V(2).Infof("Unable to parse certificate request %s in namespace %s with error %s", r.Name, r.Namespace, err)
		return
	}

	err = csr.CheckSignature()
	if err != nil {
		klog.V(2).Infof("Invalid certificate signature in CSR Request %s in namespace %s. Error %s", r.Name, r.Namespace, err)
		return
	}
	c.Spec.Request = &CsrFeatures{}
	c.Spec.Request.ValidSignature = err == nil
	c.Spec.Request.Subject = anonymizePkxName(csr.Subject)

	c.Spec.Request.SignatureAlgorithm = csr.SignatureAlgorithm.String()
	c.Spec.Request.PublicKeyAlgorithm = csr.PublicKeyAlgorithm.String()
	c.Spec.Request.DNSNames = Map(csr.DNSNames, anonymizeURL)
	c.Spec.Request.EmailAddresses = Map(csr.EmailAddresses, anonymizeURL)
	ipsl := make([]string, len(csr.IPAddresses))
	for i, ip := range csr.IPAddresses {
		ipsl[i] = ip.String()
	}
	c.Spec.Request.IPAddresses = Map(ipsl, anonymizeURL)
	urlsl := make([]string, len(csr.URIs))
	for i, u := range csr.URIs {
		urlsl[i] = u.String()
	}
	c.Spec.Request.URIs = Map(urlsl, anonymizeURL)
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
			*(dst[i].(*[]string)) = Map(*s, anonymizeString)
		default:
			panic(fmt.Sprintf("unknown type %T", s))
		}
	}
	return
}

// returns true if certificate is valid
func anonymizeCSRCert(r *certificatesv1b1api.CertificateSigningRequest, c *CSRAnonymizedFeatures) {
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
		klog.V(2).Infof("Unable to decode PEM Certificate block for CSR %s in namespace %s", r.Name, r.Namespace)
		return
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		klog.V(2).Infof("Unable to parse certificate %s in namespace %s with error %s", r.Name, r.Namespace, err)
		return
	}
	c.Status.Cert = &CertFeatures{}
	c.Status.Cert.Verified = cert != nil
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

func anonymizeCSR(r *certificatesv1b1api.CertificateSigningRequest) *CSRAnonymizedFeatures {
	c := &CSRAnonymizedFeatures{}
	fns := []func(r *certificatesv1b1api.CertificateSigningRequest, c *CSRAnonymizedFeatures){
		addMeta,
		anonymizeCSRRequest,
		anonymizeCSRCert,
	}
	for _, f := range fns {
		f(r, c)
	}
	return c
}

// Map applies each of functions to passed slice
func Map(it []string, fn func(string) string) []string {
	outSlice := []string{}
	for _, str := range it {
		outSlice = append(outSlice, fn(str))
	}
	return outSlice
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
