package insightsclient

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"strconv"
	"time"

	"k8s.io/client-go/transport"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/klog"

	"k8s.io/client-go/pkg/version"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/openshift/support-operator/pkg/authorizer"
)

type Client struct {
	client      *http.Client
	endpoint    string
	maxBytes    int64
	metricsName string

	authorizer  Authorizer
	clusterInfo ClusterVersionInfo
}

type Authorizer interface {
	Authorize(req *http.Request) error
	Enabled() (bool, time.Duration, string)
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

type nopAuthorizer struct{}

func (nopAuthorizer) Authorize(_ *http.Request) error        { return nil }
func (nopAuthorizer) Enabled() (bool, time.Duration, string) { return true, 0, "" }

func New(client *http.Client, defaultEndpoint string, maxBytes int64, metricsName string, authorizer Authorizer, clusterInfo ClusterVersionInfo) *Client {
	if client == nil {
		client = &http.Client{Transport: DefaultTransport()}
	}
	if maxBytes == 0 {
		maxBytes = 10 * 1024 * 1024
	}
	if authorizer == nil {
		authorizer = nopAuthorizer{}
	}
	return &Client{
		client:      client,
		endpoint:    defaultEndpoint,
		maxBytes:    maxBytes,
		metricsName: metricsName,
		authorizer:  authorizer,
		clusterInfo: clusterInfo,
	}
}

func (c *Client) Endpoint() string { return c.endpoint }

func (c *Client) Enabled() (bool, time.Duration, string) {
	return c.authorizer.Enabled()
}

func (c *Client) Send(ctx context.Context, source Source) error {
	cv := c.clusterInfo.ClusterVersion()
	if cv == nil {
		return ErrWaitingForVersion
	}

	req, err := http.NewRequest("POST", c.endpoint, nil)
	if err != nil {
		return err
	}

	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header.Set("User-Agent", fmt.Sprintf("support-operator/%s cluster/%s", version.Get().GitCommit, cv.Spec.ClusterID))
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
			return
		}
		pw.CloseWithError(mw.Close())
	}()

	req = req.WithContext(ctx)
	req.Body = pr

	klog.V(4).Infof("Uploading %s to %s", source.Type, req.URL.String())
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}

	defer func() {
		if _, err := io.Copy(ioutil.Discard, resp.Body); err != nil {
			log.Printf("error copying body: %v", err)
		}
		resp.Body.Close()
	}()

	switch resp.StatusCode {
	case http.StatusOK:
		gaugeRequestSend.WithLabelValues(c.metricsName, "200").Inc()
	case http.StatusAccepted:
		gaugeRequestSend.WithLabelValues(c.metricsName, "202").Inc()
	case http.StatusUnauthorized:
		gaugeRequestSend.WithLabelValues(c.metricsName, "401").Inc()
		return authorizer.Error{Err: fmt.Errorf("gateway server requires authentication: %s", resp.Request.URL)}
	case http.StatusForbidden:
		gaugeRequestSend.WithLabelValues(c.metricsName, "403").Inc()
		return authorizer.Error{Err: fmt.Errorf("gateway server forbidden: %s", resp.Request.URL)}
	case http.StatusBadRequest:
		gaugeRequestSend.WithLabelValues(c.metricsName, "400").Inc()
		body, _ := ioutil.ReadAll(resp.Body)
		if len(body) > 1024 {
			body = body[:1024]
		}
		return fmt.Errorf("gateway server bad request: %s: %s", resp.Request.URL, string(body))
	default:
		gaugeRequestSend.WithLabelValues(c.metricsName, strconv.Itoa(resp.StatusCode)).Inc()
		body, _ := ioutil.ReadAll(resp.Body)
		if len(body) > 1024 {
			body = body[:1024]
		}
		return fmt.Errorf("gateway server reported unexpected error code: %d: %s", resp.StatusCode, string(body))
	}

	if value := resp.Header.Get("x-rh-insights-request-id"); len(value) > 0 {
		klog.Infof("Successfully reported id=%s x-rh-insights-request-id=%s", source.ID, value)
	}

	return nil
}

func DefaultTransport() http.RoundTripper {
	return transport.DebugWrappers(&http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   true,
	})
}

var (
	gaugeRequestSend = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "insightsclient_request_send",
		Help: "Tracks the number of metrics sends",
	}, []string{"client", "status_code"})
)

func init() {
	prometheus.MustRegister(
		gaugeRequestSend,
	)
}
