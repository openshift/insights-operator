package insightsreport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/authorizer"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/controllerstatus"
	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
)

// Controller gathers the report from Smart Proxy
type Controller struct {
	controllerstatus.Simple

	configurator          Configurator
	client                *insightsclient.Client
	LastReport            SmartProxyReport
	archiveUploadReporter <-chan struct{}
}

// Response represents the Smart Proxy report response structure
type Response struct {
	Report SmartProxyReport `json:"report"`
}

// Configurator represents the interface to retrieve the configuration for the gatherer
type Configurator interface {
	Config() *config.Controller
	ConfigChanged() (<-chan struct{}, func())
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
func New(client *insightsclient.Client, configurator Configurator, reporter InsightsReporter) *Controller {
	return &Controller{
		Simple:                controllerstatus.Simple{Name: "insightsreport"},
		configurator:          configurator,
		client:                client,
		archiveUploadReporter: reporter.ArchiveUploaded(),
	}
}

// PullSmartProxy performs a request to the Smart Proxy and unmarshal the response
func (c *Controller) PullSmartProxy() (bool, error) {
	klog.Info("Pulling report from smart-proxy")
	config := c.configurator.Config()
	reportEndpoint := config.ReportEndpoint

	if len(reportEndpoint) == 0 {
		klog.V(4).Info("Not downloading report because Smart Proxy client is not properly configured: missing report endpoint")
		return true, nil
	}

	ctx, cancelFunc := context.WithTimeout(context.Background(), time.Minute)
	defer cancelFunc()

	klog.V(4).Info("Retrieving report")
	reportBody, err := c.client.RecvReport(ctx, reportEndpoint)
	if authorizer.IsAuthorizationError(err) {
		c.Simple.UpdateStatus(controllerstatus.Summary{
			Operation: controllerstatus.DownloadingReport,
			Reason:    "NotAuthorized",
			Message:   fmt.Sprintf("Auth rejected for downloading latest report: %v", err),
		})
		return true, err
	} else if err == insightsclient.ErrWaitingForVersion {
		klog.Error(err)
		return false, err
	} else if insightsclient.IsHttpError(err) {

		ie := err.(insightsclient.HttpError)
		klog.Errorf("Unexpected error retrieving the report: %s", ie)
		// if there's a 404 response then retry
		if ie.StatusCode == http.StatusNotFound {
			return false, ie
		}
		return true, ie
	} else if err != nil {
		klog.Errorf("Unexpected error retrieving the report: %s", err)
		c.Simple.UpdateStatus(controllerstatus.Summary{
			Operation: controllerstatus.DownloadingReport,
			Reason:    "UnexpectedError",
			Message:   fmt.Sprintf("Failed to download the latest report: %v", err),
		})
		return true, err
	}

	klog.V(4).Info("Report retrieved")
	reportResponse := Response{}

	if err = json.NewDecoder(*reportBody).Decode(&reportResponse); err != nil {
		klog.Error("The report response cannot be parsed")
		return true, err
	}

	klog.V(4).Info("Smart Proxy report correctly parsed")

	if c.LastReport.Meta.LastCheckedAt == reportResponse.Report.Meta.LastCheckedAt {
		klog.V(2).Info("Retrieved report is equal to previus one. Retrying...")
		return true, fmt.Errorf("report not updated")
	}

	updateInsightsMetrics(reportResponse.Report)
	c.LastReport = reportResponse.Report
	c.Simple.UpdateStatus(controllerstatus.Summary{Healthy: true})
	return true, nil
}

// RetrieveReport gets the report from Smart Proxy, if possible, handling the delays and timeouts
func (c *Controller) RetrieveReport() {
	klog.V(4).Info("Starting retrieving report from Smart Proxy")
	config := c.configurator.Config()
	configCh, cancelFn := c.configurator.ConfigChanged()
	defer cancelFn()

	if config.ReportPullingTimeout == 0 {
		klog.V(4).Info("Not downloading report because Smart Proxy client is not properly configured: missing polling timeout")
		return
	}

	delay := config.ReportPullingDelay
	klog.V(4).Infof("Initial delay for pulling: %v", delay)
	startTime := time.Now()
	delayTimer := time.NewTimer(wait.Jitter(delay, 0.1))
	timeoutTimer := time.NewTimer(config.ReportPullingTimeout)
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
					klog.V(4).Info("Report retrieved correctly")
				}
				return
			}

			firstPullDone = true
			if retryCounter >= retryThreshold {
				c.Simple.UpdateStatus(controllerstatus.Summary{
					Operation: controllerstatus.DownloadingReport,
					Reason:    "NotAvailable",
					Message:   fmt.Sprintf("Couldn't download the latest report: %v", err),
				})
				return
			}
			t := wait.Jitter(config.ReportMinRetryTime, 0.1)
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
				newDeadline := iterationStart.Add(config.ReportMinRetryTime)
				nextTick = wait.Jitter(time.Until(newDeadline), 0.3)
			} else {
				newDeadline := iterationStart.Add(config.ReportPullingDelay)
				nextTick = wait.Jitter(time.Until(newDeadline), 0.1)
			}

			if !delayTimer.Stop() {
				<-delayTimer.C
			}
			delayTimer.Reset(nextTick)

			// Update pulling timeout
			newTimeoutEnd := startTime.Add(config.ReportPullingTimeout)
			if !timeoutTimer.Stop() {
				<-timeoutTimer.C
			}
			timeoutTimer.Reset(time.Until(newTimeoutEnd))
		}
	}
}

// Run goroutine code for gathering the reports from Smart Proxy
func (c *Controller) Run(ctx context.Context) {
	c.Simple.UpdateStatus(controllerstatus.Summary{Healthy: true})
	klog.V(2).Info("Starting report retriever")
	klog.V(2).Infof("Initial config: %v", c.configurator.Config())

	for {
		// always wait for new uploaded archive or insights-operator ends
		select {
		case <-c.archiveUploadReporter:
			klog.V(4).Info("Archive uploaded, starting pulling report...")
			c.RetrieveReport()

		case <-ctx.Done():
			return
		}
	}
}

// updateInsightsMetrics update the Prometheus metrics from a report
func updateInsightsMetrics(report SmartProxyReport) {
	var critical, important, moderate, low, total int

	total = report.Meta.Count

	for _, rule := range report.Data {
		switch rule.TotalRisk {
		case 1:
			low++
		case 2:
			moderate++
		case 3:
			important++
		case 4:
			critical++
		}
	}

	insightsStatus.WithLabelValues("low").Set(float64(low))
	insightsStatus.WithLabelValues("moderate").Set(float64(moderate))
	insightsStatus.WithLabelValues("important").Set(float64(important))
	insightsStatus.WithLabelValues("critical").Set(float64(critical))
	insightsStatus.WithLabelValues("total").Set(float64(total))
}

func init() {
	err := legacyregistry.Register(insightsStatus)
	if err != nil {
		fmt.Println(err)
	}

	insightsStatus.WithLabelValues("low").Set(float64(-1))
	insightsStatus.WithLabelValues("moderate").Set(float64(-1))
	insightsStatus.WithLabelValues("important").Set(float64(-1))
	insightsStatus.WithLabelValues("critical").Set(float64(-1))
	insightsStatus.WithLabelValues("total").Set(float64(-1))
}
