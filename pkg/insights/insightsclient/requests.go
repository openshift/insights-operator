package insightsclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"

	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/authorizer"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// Send uploads archives to Ingress service
func (c *Client) Send(ctx context.Context, endpoint string, source Source) error {
	cv, err := c.GetClusterVersion()
	if apierrors.IsNotFound(err) {
		return ErrWaitingForVersion
	}
	if err != nil {
		return err
	}

	req, err := c.prepareRequest(ctx, http.MethodPost, endpoint, cv)
	if err != nil {
		return err
	}

	bytesRead := make(chan int64, 1)
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	go c.createAndWriteMIMEHeader(&source, mw, pw, bytesRead)
	req.Body = pr
	// dynamically set the proxy environment
	c.client.Transport = clientTransport(c.authorizer)

	klog.V(4).Infof("Uploading %s to %s", source.Type, req.URL.String())
	resp, err := c.client.Do(req)
	if err != nil {
		klog.V(4).Infof("Unable to build a request, possible invalid token: %v", err)
		// if the request is not build, for example because of invalid endpoint,(maybe some problem with DNS), we want to have record about it in metrics as well.
		counterRequestSend.WithLabelValues(c.metricsName, "0").Inc()
		return fmt.Errorf("unable to build request to connect to Insights server: %v", err)
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

	counterRequestSend.WithLabelValues(c.metricsName, strconv.Itoa(resp.StatusCode)).Inc()

	if resp.StatusCode == http.StatusUnauthorized {
		klog.V(2).Infof("gateway server %s returned 401, %s=%s", resp.Request.URL, insightsReqId, requestID)
		return authorizer.Error{Err: fmt.Errorf("your Red Hat account is not enabled for remote support or your token has expired: %s", responseBody(resp))}
	}

	if resp.StatusCode == http.StatusForbidden {
		klog.V(2).Infof("gateway server %s returned 403, %s=%s", resp.Request.URL, insightsReqId, requestID)
		return authorizer.Error{Err: fmt.Errorf("your Red Hat account is not enabled for remote support")}
	}

	if resp.StatusCode == http.StatusBadRequest {
		return fmt.Errorf("gateway server bad request: %s (request=%s): %s", resp.Request.URL, requestID, responseBody(resp))
	}

	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		return fmt.Errorf("gateway server reported unexpected error code: %d (request=%s): %s", resp.StatusCode, requestID, responseBody(resp))
	}

	if len(requestID) > 0 {
		klog.V(2).Infof("Successfully reported id=%s %s=%s, wrote=%d", source.ID, insightsReqId, requestID, <-bytesRead)
	}

	return nil
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

	klog.V(4).Infof("Retrieving report from %s", req.URL.String())
	resp, err := c.client.Do(req)

	if err != nil {
		klog.Errorf("Unable to retrieve latest report for %s: %v", cv.Spec.ClusterID, err)
		counterRequestRecvReport.WithLabelValues(c.metricsName, "0").Inc()
		return nil, err
	}

	requestID := resp.Header.Get("x-rh-insights-request-id")

	if resp.StatusCode == http.StatusUnauthorized {
		klog.V(2).Infof("gateway server %s returned 401, x-rh-insights-request-id=%s", resp.Request.URL, requestID)
		c.IncrementRecvReportMetric(resp.StatusCode)
		return nil, authorizer.Error{Err: fmt.Errorf("your Red Hat account is not enabled for remote support or your token has expired")}
	}

	if resp.StatusCode == http.StatusForbidden {
		klog.V(2).Infof("gateway server %s returned 403, x-rh-insights-request-id=%s", resp.Request.URL, requestID)
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
	return nil, fmt.Errorf("Report response status code: %d", resp.StatusCode)
}

func (c *Client) RecvSCACerts(_ context.Context, endpoint string) ([]byte, error) {
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
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer([]byte(scaArchPayload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.client.Transport = clientTransport(c.authorizer)
	authHeader := fmt.Sprintf("AccessToken %s:%s", cv.Spec.ClusterID, token)
	req.Header.Set("Authorization", authHeader)

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

// RecvGatheringRules performs a request to Insights Operator Gathering Conditions Service
// https://github.com/RedHatInsights/insights-operator-gathering-conditions-service
// and returns the response body or an error
func (c *Client) RecvGatheringRules(ctx context.Context, endpoint string) ([]byte, error) {
	klog.Infof(
		`Preparing a request to Insights Operator Gathering Conditions Service at the endpoint "%v"`, endpoint,
	)
	cv, err := c.GetClusterVersion()
	if apierrors.IsNotFound(err) {
		return nil, ErrWaitingForVersion
	}
	if err != nil {
		return nil, err
	}

	req, err := c.prepareRequest(ctx, http.MethodGet, endpoint, cv)
	if err != nil {
		return nil, err
	}

	// dynamically set the proxy environment and authentication
	c.client.Transport = clientTransport(c.authorizer)

	klog.Infof("Performing a request to Insights Operator Gathering Conditions Service")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, newHTTPErrorFromResponse(resp)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			klog.Warningf("failed to close response body: %v", err)
		}
	}()

	return io.ReadAll(resp.Body)
}

// RecvClusterTransfer performs a request to the OCM cluster transfer API.
// It is a HTTP GET request with the `search` query parameter limiting the result
// only for the one cluster and only for the `accepted` cluster transfers.
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
