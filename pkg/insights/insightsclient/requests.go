package insightsclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/authorizer"
	"github.com/openshift/insights-operator/pkg/insights"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

var (
	// when there's no HTTP status response code then we send 0 to track
	// this request in the "insightsclient_request_send_total" Prometheus metrics
	noHttpStatusCode = 0
)

func (c *Client) SendAndGetID(ctx context.Context, endpoint string, source Source) (string, int, error) {
	cv, err := c.GetClusterVersion()
	if apierrors.IsNotFound(err) {
		return "", noHttpStatusCode, ErrWaitingForVersion
	}
	if err != nil {
		return "", noHttpStatusCode, err
	}

	req, err := c.prepareRequest(ctx, http.MethodPost, endpoint, cv)
	if err != nil {
		return "", noHttpStatusCode, err
	}

	bytesRead := make(chan int64, 1)
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	go c.createAndWriteMIMEHeader(&source, mw, pw, bytesRead)
	req.Body = pr
	// dynamically set the proxy environment
	c.client.Transport = clientTransport(c.authorizer)

	klog.Infof("Uploading %s to %s", source.Type, req.URL.String())
	resp, err := c.client.Do(req)
	if err != nil {
		klog.Infof("Unable to build a request, possible invalid token: %v", err)
		// if the request is not build, for example because of invalid endpoint,(maybe some problem with DNS), we want to have record about it in metrics as well.
		insights.IncrementCounterRequestSend(noHttpStatusCode)
		return "", noHttpStatusCode, fmt.Errorf("unable to build request to connect to Insights server: %v", err)
	}

	requestID := resp.Header.Get(insightsReqId)

	defer func() {
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			klog.Warningf("error copying body: %v", err)
		}
		if err := resp.Body.Close(); err != nil {
			klog.Warningf("Failed to close response body: %v", err)
		}
	}()

	insights.IncrementCounterRequestSend(resp.StatusCode)

	if resp.StatusCode == http.StatusUnauthorized {
		klog.Infof("gateway server %s returned 401, %s=%s", resp.Request.URL, insightsReqId, requestID)
		return "", resp.StatusCode, authorizer.Error{Err: fmt.Errorf("your Red Hat account is not enabled for remote support or your token has expired: %s", responseBody(resp))}
	}

	if resp.StatusCode == http.StatusForbidden {
		klog.Infof("gateway server %s returned 403, %s=%s", resp.Request.URL, insightsReqId, requestID)
		return "", resp.StatusCode, authorizer.Error{Err: fmt.Errorf("your Red Hat account is not enabled for remote support")}
	}

	if resp.StatusCode == http.StatusBadRequest {
		return "", resp.StatusCode, fmt.Errorf("gateway server bad request: %s (request=%s): %s", resp.Request.URL, requestID, responseBody(resp))
	}

	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		return "", resp.StatusCode, fmt.Errorf("gateway server reported unexpected error code: %d (request=%s): %s", resp.StatusCode, requestID, responseBody(resp))
	}

	if len(requestID) > 0 {
		klog.Infof("Successfully reported id=%s %s=%s, wrote=%d", source.ID, insightsReqId, requestID, <-bytesRead)
	}

	return requestID, resp.StatusCode, nil
}

// Send uploads archives to Ingress service
func (c *Client) Send(ctx context.Context, endpoint string, source Source) error {
	_, _, err := c.SendAndGetID(ctx, endpoint, source)
	return err
}

// RecvReport performs a request to Insights Results Smart Proxy endpoint
func (c *Client) RecvReport(ctx context.Context, endpoint string) (*http.Response, error) {
	cv, err := c.GetClusterVersion()
	if apierrors.IsNotFound(err) {
		return nil, ErrWaitingForVersion
	}
	if err != nil {
		return nil, err
	}

	endpoint = fmt.Sprintf(endpoint, cv.Spec.ClusterID)
	klog.Infof("Retrieving report for cluster: %s", cv.Spec.ClusterID)
	klog.Infof("Endpoint: %s", endpoint)

	req, err := c.prepareRequest(ctx, http.MethodGet, endpoint, cv)
	if err != nil {
		return nil, err
	}

	// dynamically set the proxy environment
	c.client.Transport = clientTransport(c.authorizer)

	klog.Infof("Retrieving report from %s", req.URL.String())
	resp, err := c.client.Do(req)

	if err != nil {
		klog.Errorf("Unable to retrieve latest report for %s: %v", cv.Spec.ClusterID, err)
		counterRequestRecvReport.WithLabelValues(c.metricsName, "0").Inc()
		return nil, err
	}

	requestID := resp.Header.Get("x-rh-insights-request-id")

	if resp.StatusCode == http.StatusUnauthorized {
		klog.Infof("gateway server %s returned 401, x-rh-insights-request-id=%s", resp.Request.URL, requestID)
		c.IncrementRecvReportMetric(resp.StatusCode)
		return nil, authorizer.Error{Err: fmt.Errorf("your Red Hat account is not enabled for remote support or your token has expired")}
	}

	if resp.StatusCode == http.StatusForbidden {
		klog.Infof("gateway server %s returned 403, x-rh-insights-request-id=%s", resp.Request.URL, requestID)
		c.IncrementRecvReportMetric(resp.StatusCode)
		return nil, authorizer.Error{Err: fmt.Errorf("your Red Hat account is not enabled for remote support")}
	}

	if resp.StatusCode == http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 1024 {
			body = body[:1024]
		}
		c.IncrementRecvReportMetric(resp.StatusCode)
		return nil, fmt.Errorf("gateway server bad request: %s (request=%s): %s", resp.Request.URL, requestID, string(body))
	}
	if resp.StatusCode == http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 1024 {
			body = body[:1024]
		}
		notFoundErr := HttpError{
			StatusCode: resp.StatusCode,
			Err:        fmt.Errorf("not found: %s (request=%s): %s", resp.Request.URL, requestID, string(body)),
		}
		c.IncrementRecvReportMetric(resp.StatusCode)
		return nil, notFoundErr
	}

	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 1024 {
			body = body[:1024]
		}
		c.IncrementRecvReportMetric(resp.StatusCode)
		return nil, fmt.Errorf("gateway server reported unexpected error code: %d (request=%s): %s", resp.StatusCode, requestID, string(body))
	}

	if resp.StatusCode == http.StatusOK {
		return resp, nil
	}

	klog.Warningf("Report response status code: %d", resp.StatusCode)
	return nil, fmt.Errorf("report response status code: %d", resp.StatusCode)
}

func (c *Client) RecvSCACerts(_ context.Context, endpoint string, architecture string) ([]byte, error) {
	cv, err := c.GetClusterVersion()
	if apierrors.IsNotFound(err) {
		return nil, ErrWaitingForVersion
	}
	if err != nil {
		return nil, err
	}
	token, err := c.authorizer.Token()
	if err != nil {
		return nil, err
	}
	payload := fmt.Sprintf(scaArchPayload, architecture)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer([]byte(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.client.Transport = clientTransport(c.authorizer)
	authHeader := fmt.Sprintf("AccessToken %s:%s", cv.Spec.ClusterID, token)
	req.Header.Set("Authorization", authHeader)
	klog.Infof("Asking for SCA certificate for %s architecture", architecture)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve SCA certs data from %s: %v", endpoint, err)
	}

	if resp.StatusCode > 399 || resp.StatusCode < 200 {
		return nil, ocmErrorMessage(resp)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			klog.Warningf("Failed to close response body: %v", err)
		}
	}()

	return io.ReadAll(resp.Body)
}

// RecvClusterTransfer performs a request to the OCM cluster transfer API. It is
// an HTTP GET request with the `search` query parameter limiting the result only
// for the one cluster and only for the `accepted` cluster transfers.
func (c *Client) RecvClusterTransfer(endpoint string) ([]byte, error) {
	cv, err := c.GetClusterVersion()
	if apierrors.IsNotFound(err) {
		return nil, ErrWaitingForVersion
	}
	if err != nil {
		return nil, err
	}
	token, err := c.authorizer.Token()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	searchQuery := fmt.Sprintf("cluster_uuid is '%s' and status is 'accepted'", cv.Spec.ClusterID)
	q.Add("search", searchQuery)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Content-Type", "application/json")
	c.client.Transport = clientTransport(c.authorizer)
	authHeader := fmt.Sprintf("AccessToken %s:%s", cv.Spec.ClusterID, token)
	req.Header.Set("Authorization", authHeader)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve cluster transfer data from %s: %v", endpoint, err)
	}

	if resp.StatusCode > 399 || resp.StatusCode < 200 {
		return nil, ocmErrorMessage(resp)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			klog.Warningf("Failed to close response body: %v", err)
		}
	}()
	return io.ReadAll(resp.Body)
}

// GetWithPathParam makes an HTTP GET request to the specified endpoint using the specified "params" as
// a part of the endpoint path
func (c *Client) GetWithPathParam(ctx context.Context, endpoint, param string, includeClusterID bool) (*http.Response, error) {
	cv, err := c.GetClusterVersion()
	if apierrors.IsNotFound(err) {
		return nil, ErrWaitingForVersion
	}
	if err != nil {
		return nil, err
	}
	if includeClusterID {
		endpoint = fmt.Sprintf(endpoint, cv.Spec.ClusterID, param)
	} else {
		endpoint = fmt.Sprintf(endpoint, param)
	}
	klog.Infof("Making HTTP GET request at: %s", endpoint)

	req, err := c.prepareRequest(ctx, http.MethodGet, endpoint, cv)
	if err != nil {
		return nil, err
	}

	// dynamically set the proxy environment
	c.client.Transport = clientTransport(c.authorizer)

	return c.client.Do(req)
}
