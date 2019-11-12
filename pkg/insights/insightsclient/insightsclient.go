package insightsclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"strconv"
	"time"

	knet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/client-go/pkg/version"
	"k8s.io/client-go/transport"
	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"

	"k8s.io/klog"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/openshift/insights-operator/pkg/authorizer"
)

type Client struct {
	client      *http.Client
	maxBytes    int64
	metricsName string

	authorizer  Authorizer
	clusterInfo ClusterVersionInfo
}

type Authorizer interface {
	Authorize(req *http.Request) error
}

type ClusterVersionInfo interface {
	ClusterVersion() *configv1.ClusterVersion
}

type Source struct {
	ID       string
	Type     string
	Contents io.Reader
}

var ErrWaitingForVersion = fmt.Errorf("waiting for the cluster version to be loaded")

func New(client *http.Client, maxBytes int64, metricsName string, authorizer Authorizer, clusterInfo ClusterVersionInfo) *Client {
	if client == nil {
		client = &http.Client{}
	}
	if maxBytes == 0 {
		maxBytes = 10 * 1024 * 1024
	}
	return &Client{
		client:      client,
		maxBytes:    maxBytes,
		metricsName: metricsName,
		authorizer:  authorizer,
		clusterInfo: clusterInfo,
	}
}

func getTrustedCABundle() (*x509.CertPool, error) {
	caBytes, err := ioutil.ReadFile("/var/run/configmaps/trusted-ca-bundle/ca-bundle.crt")
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

func clientTransport() http.RoundTripper {
	// default transport, proxy from env
	clientTransport := &http.Transport{
		Proxy: knet.NewProxierWithNoProxyCIDR(http.ProxyFromEnvironment),
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

func (c *Client) Send(ctx context.Context, endpoint string, source Source) error {
	cv := c.clusterInfo.ClusterVersion()
	if cv == nil {
		return ErrWaitingForVersion
	}

	req, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		return err
	}

	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header.Set("User-Agent", fmt.Sprintf("insights-operator/%s cluster/%s", version.Get().GitCommit, cv.Spec.ClusterID))
	if err := c.authorizer.Authorize(req); err != nil {
		return err
	}

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	go func() {
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, "file", "payload.tar.gz"))
		h.Set("Content-Type", source.Type)
		fw, err := mw.CreatePart(h)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		r := &LimitedReader{R: source.Contents, N: c.maxBytes}
		if _, err := io.Copy(fw, r); err != nil {
			pw.CloseWithError(err)
		}
		pw.CloseWithError(mw.Close())
	}()

	req = req.WithContext(ctx)
	req.Body = pr

	// dynamically set the proxy environment
	c.client.Transport = clientTransport()

	klog.V(4).Infof("Uploading %s to %s", source.Type, req.URL.String())
	resp, err := c.client.Do(req)
	if err != nil {
		klog.V(4).Infof("Unable to build a request, possible invalid token: %v", err)
		return fmt.Errorf("unable to build request to connect to Insights server")
	}

	requestID := resp.Header.Get("x-rh-insights-request-id")

	defer func() {
		if _, err := io.Copy(ioutil.Discard, resp.Body); err != nil {
			klog.Warningf("error copying body: %v", err)
		}
		if err := resp.Body.Close(); err != nil {
			klog.Warningf("Failed to close response body: %v", err)
		}
	}()

	switch resp.StatusCode {
	case http.StatusOK:
		counterRequestSend.WithLabelValues(c.metricsName, "200").Inc()
	case http.StatusAccepted:
		counterRequestSend.WithLabelValues(c.metricsName, "202").Inc()
	case http.StatusUnauthorized:
		counterRequestSend.WithLabelValues(c.metricsName, "401").Inc()
		klog.V(2).Infof("gateway server %s returned 401, x-rh-insights-request-id=%s", resp.Request.URL, requestID)
		return authorizer.Error{Err: fmt.Errorf("your Red Hat account is not enabled for remote support or your token has expired")}
	case http.StatusForbidden:
		counterRequestSend.WithLabelValues(c.metricsName, "403").Inc()
		klog.V(2).Infof("gateway server %s returned 403, x-rh-insights-request-id=%s", resp.Request.URL, requestID)
		return authorizer.Error{Err: fmt.Errorf("your Red Hat account is not enabled for remote support")}
	case http.StatusBadRequest:
		counterRequestSend.WithLabelValues(c.metricsName, "400").Inc()
		body, _ := ioutil.ReadAll(resp.Body)
		if len(body) > 1024 {
			body = body[:1024]
		}
		return fmt.Errorf("gateway server bad request: %s (request=%s): %s", resp.Request.URL, requestID, string(body))
	default:
		counterRequestSend.WithLabelValues(c.metricsName, strconv.Itoa(resp.StatusCode)).Inc()
		body, _ := ioutil.ReadAll(resp.Body)
		if len(body) > 1024 {
			body = body[:1024]
		}
		return fmt.Errorf("gateway server reported unexpected error code: %d (request=%s): %s", resp.StatusCode, requestID, string(body))
	}

	if len(requestID) > 0 {
		klog.V(2).Infof("Successfully reported id=%s x-rh-insights-request-id=%s", source.ID, requestID)
	}

	return nil
}

var (
	counterRequestSend = metrics.NewCounterVec(&metrics.CounterOpts{
		Name: "insightsclient_request_send_total",
		Help: "Tracks the number of metrics sends",
	}, []string{"client", "status_code"})
)

func init() {
	err := legacyregistry.Register(
		counterRequestSend,
	)
	if err != nil {
		fmt.Println(err)
	}
	//counterRequestSend.WithLabelValues("metric_init", "000").Add(0)

}
