package insightsclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"k8s.io/client-go/pkg/version"
	"k8s.io/client-go/transport"
	"k8s.io/component-base/metrics"

	"k8s.io/klog/v2"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned"
	"github.com/openshift/insights-operator/pkg/insights"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachineryversion "k8s.io/apimachinery/pkg/version"
)

const (
	responseBodyLogLen = 1024
	insightsReqId      = "x-rh-insights-request-id"
	scaArchPayload     = `{"type": "sca","arch": "%s"}`
)

type Client struct {
	client       *http.Client
	maxBytes     int64
	metricsName  string
	authorizer   Authorizer
	configClient *configv1client.Clientset
}

type Authorizer interface {
	Authorize(req *http.Request) error
	NewSystemOrConfiguredProxy() func(*http.Request) (*url.URL, error)
	Token() (string, error)
}

type Source struct {
	ID           string
	Type         string
	CreationTime time.Time
	Contents     io.ReadCloser
}

// HttpError is helper error type to have HTTP error status code
type HttpError struct {
	Err        error
	StatusCode int
}

// createAPIErrorMessage creates an error from http.Response combining request URL, http status
// and the body into a string
func newHTTPErrorFromResponse(r *http.Response) *HttpError {
	err := fmt.Errorf(`URL "%s" returned HTTP code %d: %s`, r.Request.URL, r.StatusCode, responseBody(r))
	return &HttpError{
		Err:        err,
		StatusCode: r.StatusCode,
	}
}

func (e HttpError) Error() string {
	return e.Err.Error()
}

func IsHttpError(err error) bool {
	switch err.(type) {
	case HttpError:
		return true
	default:
		return false
	}
}

var ErrWaitingForVersion = fmt.Errorf("waiting for the cluster version to be loaded")

// New creates a Client
func New(client *http.Client, maxBytes int64, metricsName string, authorizer Authorizer, configClient *configv1client.Clientset) *Client {
	if client == nil {
		client = &http.Client{}
	}
	if maxBytes == 0 {
		maxBytes = 10 * 1024 * 1024
	}
	return &Client{
		client:       client,
		maxBytes:     maxBytes,
		metricsName:  metricsName,
		authorizer:   authorizer,
		configClient: configClient,
	}
}

func getTrustedCABundle() (*x509.CertPool, error) {
	caBytes, err := os.ReadFile("/var/run/configmaps/trusted-ca-bundle/ca-bundle.crt")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(caBytes) == 0 {
		return nil, nil
	}
	certs := x509.NewCertPool()
	if ok := certs.AppendCertsFromPEM(caBytes); !ok {
		return nil, errors.New("error loading cert pool from ca data")
	}
	return certs, nil
}

// clientTransport creates new http.Transport with either system or configured Proxy
func clientTransport(authorizer Authorizer) http.RoundTripper {
	clientTransport := &http.Transport{
		Proxy: authorizer.NewSystemOrConfiguredProxy(),
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   true,
	}

	// get the cluster proxy trusted CA bundle in case the proxy need it
	rootCAs, err := getTrustedCABundle()
	if err != nil {
		klog.Errorf("Failed to get proxy trusted CA: %v", err)
	}
	if rootCAs != nil {
		clientTransport.TLSClientConfig = &tls.Config{}
		clientTransport.TLSClientConfig.RootCAs = rootCAs
	}

	return transport.DebugWrappers(clientTransport)
}

func userAgent(releaseVersionEnv string, v apimachineryversion.Info, cv *configv1.ClusterVersion) string {
	gitVersion := v.GitVersion
	// If the RELEASE_VERSION is set in pod, use it
	if releaseVersionEnv != "" {
		gitVersion = releaseVersionEnv
	}
	gitVersion = fmt.Sprintf("%s-%s", gitVersion, v.GitCommit)
	return fmt.Sprintf("insights-operator/%s cluster/%s", gitVersion, cv.Spec.ClusterID)
}

func (c *Client) GetClusterVersion() (*configv1.ClusterVersion, error) {
	ctx := context.Background()

	cv, err := c.configClient.ConfigV1().ClusterVersions().Get(ctx, "version", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return cv, nil
}

func (c *Client) prepareRequest(ctx context.Context, method string, endpoint string, cv *configv1.ClusterVersion) (*http.Request, error) {
	req, err := http.NewRequest(method, endpoint, nil)
	if err != nil {
		return nil, err
	}

	if req.Header == nil {
		req.Header = make(http.Header)
	}

	releaseVersionEnv := os.Getenv("RELEASE_VERSION")
	ua := userAgent(releaseVersionEnv, version.Get(), cv)
	req.Header.Set("User-Agent", ua)
	if err := c.authorizer.Authorize(req); err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	return req, nil
}

func responseBody(r *http.Response) string {
	if r == nil {
		return ""
	}
	body, _ := io.ReadAll(r.Body)
	if len(body) > responseBodyLogLen {
		body = body[:responseBodyLogLen]
	}
	return string(body)
}

// ocmErrorMessage wraps the OCM error states in the error
func ocmErrorMessage(r *http.Response) error {
	requestURL := r.Request.URL
	err := fmt.Errorf("OCM API %s returned HTTP %d: %s", requestURL, r.StatusCode, responseBody(r))
	return HttpError{
		Err:        err,
		StatusCode: r.StatusCode,
	}
}

// IncrementRecvReportMetric increments the "insightsclient_request_recvreport_total" metric for the given HTTP status code
func (c *Client) IncrementRecvReportMetric(statusCode int) {
	counterRequestRecvReport.WithLabelValues(c.metricsName, strconv.Itoa(statusCode)).Inc()
}

// createAndWriteMIMEHeader creates and writes a new MIME header. There are two parts (basically two content-disposition headers).
// First is to write the tar.gz file and second is to write `custom_metadata` field including gathering time info. Both parts are
// written with the provided `multipart.Writer`.
func (c *Client) createAndWriteMIMEHeader(source *Source, mw *multipart.Writer, pw *io.PipeWriter, ch chan<- int64) {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, "file", "payload.tar.gz"))
	h.Set("Content-Type", source.Type)
	fw, err := mw.CreatePart(h)
	if err != nil {
		_ = pw.CloseWithError(err)
		return
	}
	r := &LimitedReader{R: source.Contents, N: c.maxBytes}
	n, err := io.Copy(fw, r)
	ch <- n
	if err != nil {
		_ = pw.CloseWithError(err)
	}
	// set gathering time as custom metadata field
	fw, err = mw.CreateFormFile("metadata", "metadata.json")
	if err != nil {
		_ = pw.CloseWithError(err)
		return
	}
	cm := fmt.Sprintf(`{"custom_metadata":{"gathering_time":%q}}`, source.CreationTime.Format(time.RFC3339))
	_, err = io.Copy(fw, strings.NewReader(cm))
	if err != nil {
		_ = pw.CloseWithError(err)
	}
	_ = pw.CloseWithError(mw.Close())
}

var (
	counterRequestRecvReport = metrics.NewCounterVec(&metrics.CounterOpts{
		Name: "insightsclient_request_recvreport_total",
		Help: "Tracks the number of insights reports received/downloaded",
	}, []string{"client", "status_code"})
)

func init() {
	insights.MustRegisterMetrics(counterRequestRecvReport)
}
