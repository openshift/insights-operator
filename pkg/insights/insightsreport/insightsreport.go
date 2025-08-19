package insightsreport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/component-base/metrics"
	"k8s.io/klog/v2"

	configv1 "github.com/openshift/api/config/v1"
	v1 "github.com/openshift/api/operator/v1"
	operatorv1client "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1"
	"github.com/openshift/insights-operator/pkg/authorizer"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/controllerstatus"
	"github.com/openshift/insights-operator/pkg/insights"
	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
	"github.com/openshift/insights-operator/pkg/insights/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	minReportRetryTime    = 30 * time.Second
	reportDownloadTimeout = 5 * time.Minute
)

// Controller gathers the report from Smart Proxy
type Controller struct {
	controllerstatus.StatusController

	configurator          configobserver.Interface
	client                insightsReportClient
	LastReport            types.SmartProxyReport
	archiveUploadReporter <-chan struct{}
	insightsOperatorCLI   operatorv1client.InsightsOperatorInterface
}

// InsightsReporter represents an object that can notify about archive uploading
type InsightsReporter interface {
	ArchiveUploaded() <-chan struct{}
}

var (
	// insightsStatus contains a metric with the latest report information
	insightsStatus = metrics.NewGaugeVec(&metrics.GaugeOpts{
		Namespace: "health",
		Subsystem: "statuses",
		Name:      "insights",
		Help:      "Information about the cluster health status as detected by Insights tooling.",
	}, []string{"metric"})
	// number of pulling report retries
	retryThreshold = 2
)

// New initializes and returns a Gatherer
func New(client *insightsclient.Client, configurator configobserver.Interface, reporter InsightsReporter, insightsOperatorCLI operatorv1client.InsightsOperatorInterface) *Controller {
	return &Controller{
		StatusController:      controllerstatus.New("insightsreport"),
		configurator:          configurator,
		client:                client,
		archiveUploadReporter: reporter.ArchiveUploaded(),
		insightsOperatorCLI:   insightsOperatorCLI,
	}
}

func NewWithTechPreview(client *insightsclient.Client, configurator configobserver.Interface) *Controller {
	return &Controller{
		StatusController: controllerstatus.New("insightsreport"),
		configurator:     configurator,
		client:           client,
	}
}

// PullReportTechpreview queries the "reportEndpointTechPreview" endpoint with provided Insights request ID
// to get the corresponding Insights analysis report. When the response is successfully decoded, the "insightsclient_request_recvreport_total"
// Prometheus metric is incremented and InsightsAnalysisReport is returned. This is called only in TechPreview clusters.
func (c *Controller) PullReportTechpreview(insightsRequestID string) (*types.InsightsAnalysisReport, error) {
	klog.Info("Retrieving report from the insights-results-agregator service endpoint")
	config := c.configurator.Config()
	reportEndpointTP := config.DataReporting.DownloadEndpointTechPreview

	if len(reportEndpointTP) == 0 {
		klog.Info("Not downloading report because the insights-results-agregator endpoint is not configured")
		c.StatusController.UpdateStatus(controllerstatus.Summary{Healthy: true})
		return nil, nil
	}

	ctx, cancelFunc := context.WithTimeout(context.Background(), time.Minute)
	defer cancelFunc()

	resp, err := c.client.GetWithPathParam(ctx, reportEndpointTP, insightsRequestID, true)
	if err != nil {
		klog.Errorf("Unexpected error retrieving the report: %v", err)
		c.UpdateStatus(controllerstatus.Summary{
			Healthy:   false,
			Operation: controllerstatus.DownloadingReport,
			Message:   fmt.Sprintf("Couldn't download the latest report: %v", err),
			Reason:    "UnexpectedError",
		})
		return nil, err
	}
	downloadTime := metav1.Now()
	c.client.IncrementRecvReportMetric(resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		errFmt := "Failed to download the latest report: HTTP %d %s"

		c.UpdateStatus(controllerstatus.Summary{
			Healthy:   false,
			Operation: controllerstatus.DownloadingReport,
			Message:   fmt.Sprintf(errFmt, resp.StatusCode, resp.Status),
			Reason:    "NotAvailable",
		})
		return nil, fmt.Errorf(errFmt, resp.StatusCode, resp.Status)
	}
	klog.Info("Report retrieved correctly")
	analysisReport := &types.InsightsAnalysisReport{}

	if err = json.NewDecoder(resp.Body).Decode(analysisReport); err != nil {
		klog.Errorf("The report response cannot be parsed: %v", err)
		return nil, err
	}
	analysisReport.DownloadedAt = downloadTime
	recommendations, healthStatus := c.readInsightsReportTechPreview(*analysisReport)
	updateInsightsMetrics(recommendations, healthStatus)
	c.StatusController.UpdateStatus(controllerstatus.Summary{Healthy: true})
	return analysisReport, nil
}

// PullSmartProxy performs a request to the Smart Proxy and unmarshal the response
func (c *Controller) PullSmartProxy() (bool, error) {
	klog.Info("Pulling report from smart-proxy")
	config := c.configurator.Config()
	reportEndpoint := config.DataReporting.DownloadEndpoint

	if len(reportEndpoint) == 0 {
		klog.Info("Not downloading report because Smart Proxy client is not properly configured: missing report endpoint")
		return true, nil
	}

	ctx, cancelFunc := context.WithTimeout(context.Background(), time.Minute)
	defer cancelFunc()

	klog.Info("Retrieving report")
	resp, err := c.client.RecvReport(ctx, reportEndpoint)
	if authorizer.IsAuthorizationError(err) {
		c.StatusController.UpdateStatus(controllerstatus.Summary{
			Operation: controllerstatus.DownloadingReport,
			Reason:    "NotAuthorized",
			Message:   fmt.Sprintf("Auth rejected for downloading latest report: %v", err),
		})
		return true, err
	} else if errors.Is(err, insightsclient.ErrWaitingForVersion) {
		klog.Error(err)
		return false, err
	} else if insightsclient.IsHttpError(err) {
		ie := err.(insightsclient.HttpError)
		klog.Errorf("Unexpected error retrieving the report: %v", ie)
		// if there's a 404 response then retry
		if ie.StatusCode == http.StatusNotFound {
			return false, ie
		}
		return true, ie
	} else if err != nil {
		klog.Errorf("Unexpected error retrieving the report: %v", err)
		c.StatusController.UpdateStatus(controllerstatus.Summary{
			Operation: controllerstatus.DownloadingReport,
			Reason:    "UnexpectedError",
			Message:   fmt.Sprintf("Failed to download the latest report: %v", err),
		})
		return true, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			klog.Warningf("Failed to close response body: %v", err)
		}
	}()

	klog.Info("Report retrieved")
	downloadTime := metav1.Now()
	reportResponse := types.Response{}

	if err = json.NewDecoder(resp.Body).Decode(&reportResponse); err != nil {
		klog.Error("The report response cannot be parsed")
		return true, err
	}

	klog.V(4).Info("Smart Proxy report correctly parsed")

	if c.LastReport.Meta.LastCheckedAt == reportResponse.Report.Meta.LastCheckedAt {
		klog.Info("Retrieved report is equal to previus one. Retrying...")
		return true, fmt.Errorf("report not updated")
	}

	recommendations, healthStatus := c.readInsightsReport(reportResponse.Report)
	updateInsightsMetrics(recommendations, healthStatus)
	err = c.updateOperatorStatusCR(reportResponse.Report, downloadTime)
	if err != nil {
		klog.Errorf("failed to update the Insights Operator CR status: %v", err)
	}
	// we want to increment the metric only in case of download of a new report
	c.client.IncrementRecvReportMetric(resp.StatusCode)
	c.LastReport = reportResponse.Report
	c.StatusController.UpdateStatus(controllerstatus.Summary{Healthy: true})
	return true, nil
}

// RetrieveReport gets the report from Smart Proxy, if possible, handling the delays and timeouts
func (c *Controller) RetrieveReport() {
	klog.Info("Starting retrieving report from Smart Proxy")
	config := c.configurator.Config()
	configCh, cancelFn := c.configurator.ConfigChanged()
	defer cancelFn()

	delay := config.DataReporting.ReportPullingDelay
	klog.Infof("Initial delay for pulling: %v", delay)
	startTime := time.Now()
	delayTimer := time.NewTimer(wait.Jitter(delay, 0.1))
	timeoutTimer := time.NewTimer(reportDownloadTimeout)
	firstPullDone := false
	retryCounter := 0

	// select for initial delay
	for {
		iterationStart := time.Now()
		select {
		case <-delayTimer.C:
			// Get report and set new timer
			done, err := c.PullSmartProxy()
			if done {
				if err != nil {
					klog.Errorf("Unrecoverable problem retrieving the report: %v", err)
				} else {
					klog.Info("Report retrieved correctly")
				}
				return
			}

			firstPullDone = true
			if retryCounter >= retryThreshold {
				c.StatusController.UpdateStatus(controllerstatus.Summary{
					Operation: controllerstatus.DownloadingReport,
					Reason:    "NotAvailable",
					Message:   fmt.Sprintf("Couldn't download the latest report: %v", err),
				})
				return
			}
			t := wait.Jitter(minReportRetryTime, 0.1)
			klog.Infof("Reseting the delay timer to retry in %s again", t)
			delayTimer.Reset(t)
			retryCounter++
		case <-timeoutTimer.C:
			// timeout, ends
			if !delayTimer.Stop() {
				<-delayTimer.C
			}
			return

		case <-configCh:
			// Config change, update initial counter
			config = c.configurator.Config()

			// Update next deadline
			var nextTick time.Duration
			if firstPullDone {
				newDeadline := iterationStart.Add(minReportRetryTime)
				nextTick = wait.Jitter(time.Until(newDeadline), 0.3)
			} else {
				newDeadline := iterationStart.Add(config.DataReporting.ReportPullingDelay)
				nextTick = wait.Jitter(time.Until(newDeadline), 0.1)
			}

			if !delayTimer.Stop() {
				<-delayTimer.C
			}
			delayTimer.Reset(nextTick)

			// Update pulling timeout
			newTimeoutEnd := startTime.Add(reportDownloadTimeout)
			if !timeoutTimer.Stop() {
				<-timeoutTimer.C
			}
			timeoutTimer.Reset(time.Until(newTimeoutEnd))
		}
	}
}

// Run goroutine code for gathering the reports from Smart Proxy
func (c *Controller) Run(ctx context.Context) {
	c.StatusController.UpdateStatus(controllerstatus.Summary{Healthy: true})
	klog.Info("Starting report retriever")
	conf := c.configurator.Config()
	klog.Infof("Insights analysis reports will be downloaded from the %s endpoint with a delay of %s",
		conf.DataReporting.DownloadEndpoint, conf.DataReporting.ReportPullingDelay)
	for {
		// always wait for new uploaded archive or insights-operator ends
		select {
		case <-c.archiveUploadReporter:
			klog.Info("Archive uploaded, starting pulling report...")
			c.RetrieveReport()

		case <-ctx.Done():
			return
		}
	}
}

type healthStatusCounts struct {
	critical, important, moderate, low, total int
}

type insightsReportClient interface {
	RecvReport(ctx context.Context, endpoint string) (*http.Response, error)
	IncrementRecvReportMetric(statusCode int)
	GetClusterVersion() (*configv1.ClusterVersion, error)
	GetWithPathParam(ctx context.Context, endpoint, requestID string, includeClusterID bool) (*http.Response, error)
}

// readInsightsReportTechPreview reads the specified InsightsAnalysisReport and returns slice of active
// Insights recommendations and corresponding health status
func (c *Controller) readInsightsReportTechPreview(report types.InsightsAnalysisReport) ([]types.InsightsRecommendation, healthStatusCounts) {
	healthStatus := healthStatusCounts{}
	total := 0
	activeRecommendations := []types.InsightsRecommendation{}

	for _, recommendation := range report.Recommendations {
		total++
		switch recommendation.TotalRisk {
		case 1:
			healthStatus.low++
		case 2:
			healthStatus.moderate++
		case 3:
			healthStatus.important++
		case 4:
			healthStatus.critical++
		}
		if c.configurator.Config().Alerting.Disabled {
			continue
		}
		insights.RecommendationCollector.SetClusterID(configv1.ClusterID(report.ClusterID))

		activeRecommendations = append(activeRecommendations, types.InsightsRecommendation{
			RuleID:      types.RuleID(recommendation.RuleFQDN),
			ErrorKey:    recommendation.ErrorKey,
			Description: recommendation.Description,
			TotalRisk:   recommendation.TotalRisk,
		})
	}

	healthStatus.total = total
	return activeRecommendations, healthStatus
}

func (c *Controller) readInsightsReport(report types.SmartProxyReport) ([]types.InsightsRecommendation, healthStatusCounts) {
	healthStatus := healthStatusCounts{}
	healthStatus.total = report.Meta.Count
	activeRecommendations := []types.InsightsRecommendation{}

	for _, rule := range report.Data {
		if rule.Disabled {
			// total also includes disabled rules
			healthStatus.total--
			continue
		}
		switch rule.TotalRisk {
		case 1:
			healthStatus.low++
		case 2:
			healthStatus.moderate++
		case 3:
			healthStatus.important++
		case 4:
			healthStatus.critical++
		}

		if c.configurator.Config().Alerting.Disabled {
			continue
		}
		errorKeyStr, err := extractErrorKeyFromRuleData(rule)
		if err != nil {
			klog.Errorf("Unable to extract recommendation's error key: %v", err)
			continue
		}
		clusterVersion, err := c.client.GetClusterVersion()
		if err != nil {
			klog.Errorf("Unable to extract cluster version. Error: %v", err)
			continue
		}
		insights.RecommendationCollector.SetClusterID(clusterVersion.Spec.ClusterID)

		activeRecommendations = append(activeRecommendations, types.InsightsRecommendation{
			RuleID:      rule.RuleID,
			ErrorKey:    errorKeyStr,
			Description: rule.Description,
			TotalRisk:   rule.TotalRisk,
		})
	}

	return activeRecommendations, healthStatus
}

// updateInsightsMetrics update the Prometheus metrics from a report
func updateInsightsMetrics(activeRecommendations []types.InsightsRecommendation, hsCount healthStatusCounts) {
	insights.RecommendationCollector.SetActiveRecommendations(activeRecommendations)

	insightsStatus.WithLabelValues("low").Set(float64(hsCount.low))
	insightsStatus.WithLabelValues("moderate").Set(float64(hsCount.moderate))
	insightsStatus.WithLabelValues("important").Set(float64(hsCount.important))
	insightsStatus.WithLabelValues("critical").Set(float64(hsCount.critical))
	insightsStatus.WithLabelValues("total").Set(float64(hsCount.total))
}

func (c *Controller) updateOperatorStatusCR(report types.SmartProxyReport, reportDownloadTime metav1.Time) error {
	insightsOperatorCR, err := c.insightsOperatorCLI.Get(context.Background(), "cluster", metav1.GetOptions{})
	if err != nil {
		return err
	}

	updatedOperatorCR := insightsOperatorCR.DeepCopy()
	var healthChecks []v1.HealthCheck

	for _, rule := range report.Data {
		errorKey, err := extractErrorKeyFromRuleData(rule)
		if err != nil {
			klog.Errorf("Unable to extract recommendation's error key: %v", err)
			continue
		}

		ruleIDStr := strings.TrimSuffix(string(rule.RuleID), ".report")
		advisorLink, err := insights.CreateInsightsAdvisorLink(insights.RecommendationCollector.ClusterID(), ruleIDStr, errorKey)
		if err != nil {
			klog.Errorf("Failed to create console.redhat.com link: %v", err)
			continue
		}
		healthCheck := v1.HealthCheck{
			Description: rule.Description,
			TotalRisk:   int32(rule.TotalRisk),
			State:       v1.HealthCheckEnabled,
			AdvisorURI:  advisorLink,
		}

		if rule.Disabled {
			healthCheck.State = v1.HealthCheckDisabled
		}
		healthChecks = append(healthChecks, healthCheck)
	}
	updatedOperatorCR.Status.InsightsReport.DownloadedAt = reportDownloadTime
	updatedOperatorCR.Status.InsightsReport.HealthChecks = healthChecks
	_, err = c.insightsOperatorCLI.UpdateStatus(context.Background(), updatedOperatorCR, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

// extractErrorKeyFromRuleData extracts "error_key" value from the provided rule.
func extractErrorKeyFromRuleData(r types.RuleWithContentResponse) (string, error) {
	extraDataMap, ok := r.TemplateData.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unable to convert the TemplateData of rule %q in an Insights report to a map", r.RuleID)
	}

	errorKeyField, exists := extraDataMap["error_key"]
	if !exists {
		return "", fmt.Errorf("templateData of rule %q does not contain error_key", r.RuleID)
	}

	errorKeyStr, ok := errorKeyField.(string)
	if !ok {
		return "", fmt.Errorf("the error_key of TemplateData of rule %q is not a string", r.RuleID)
	}
	return errorKeyStr, nil
}

func init() {
	insights.MustRegisterMetrics(insightsStatus)

	insightsStatus.WithLabelValues("low").Set(float64(-1))
	insightsStatus.WithLabelValues("moderate").Set(float64(-1))
	insightsStatus.WithLabelValues("important").Set(float64(-1))
	insightsStatus.WithLabelValues("critical").Set(float64(-1))
	insightsStatus.WithLabelValues("total").Set(float64(-1))
}
