package insightsreport

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/RedHatInsights/insights-results-smart-proxy/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/klog"

	"github.com/openshift/insights-operator/pkg/authorizer"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/controllerstatus"
	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
)

// Gatherer gathers the report from Smart Proxy
type Gatherer struct {
	controllerstatus.Simple

	configurator          Configurator
	client                *insightsclient.Client
	LastReport            types.SmartProxyReport
	archiveUploadReporter <-chan struct{}
	insightsReport        chan struct{}
}

// Response represents the Smart Proxy report response structure
type Response struct {
	Report types.SmartProxyReport `json:"report"`
	Status string                 `json:"status"`
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
	// ErrWaitingForVersion contains an error when the cluster version is not loaded yet
	ErrWaitingForVersion = fmt.Errorf("waiting for the cluster version to be loaded")

	// insightsStatus contains a metric with the latest report information
	insightsStatus = metrics.NewGaugeVec(&metrics.GaugeOpts{
		Namespace: "health",
		Subsystem: "statuses",
		Name:      "insights",
		Help:      "Information about the cluster health status as detected by Insights tooling.",
	}, []string{"metric"})
)

// New initializes and returns a Gatherer
func New(client *insightsclient.Client, configurator Configurator, reporter InsightsReporter) *Gatherer {
	return &Gatherer{
		Simple:                controllerstatus.Simple{Name: "insightsreport"},
		configurator:          configurator,
		client:                client,
		archiveUploadReporter: reporter.ArchiveUploaded(),
	}
}

// PullSmartProxy performs a request to the Smart Proxy and unmarshal the response
func (r *Gatherer) PullSmartProxy() {
	klog.Info("Pulling report from smart-proxy")
	ctx, cancelFunc := context.WithTimeout(context.Background(), time.Minute)
	defer cancelFunc()

	klog.V(4).Info("Retrieving report")
	config := r.configurator.Config()
	reportEndpoint := config.ReportEndpoint
	reportBody, err := r.client.RecvReport(ctx, reportEndpoint)
	if authorizer.IsAuthorizationError(err) {
		r.Simple.UpdateStatus(controllerstatus.Summary{
			Operation: controllerstatus.DownloadingReport,
			Reason:    "NotAuthorized",
			Message:   fmt.Sprintf("Auth rejected for downloading latest report: %v", err),
		})
		return
	} else if err != nil {
		klog.Errorf("Error retrieving the report: %s", err)
		return
	}

	klog.V(4).Info("Report retrieved")
	reportResponse := Response{}

	if err = json.NewDecoder(*reportBody).Decode(&reportResponse); err != nil {
		klog.Error("The report response cannot be parsed")
		return
	}

	klog.V(4).Info("Smart Proxy report correctly parsed")

	if r.LastReport.Meta.LastCheckedAt == reportResponse.Report.Meta.LastCheckedAt {
		klog.V(2).Info("Retrieved report is equal to previus one. Retrying...")
		return
	}

	defer close(r.insightsReport)
	updateInsightsMetrics(reportResponse.Report)
	r.LastReport = reportResponse.Report
	return
}

// Run goroutine code for gathering the reports from Smart Proxy
func (r *Gatherer) Run(ctx context.Context) {
	r.Simple.UpdateStatus(controllerstatus.Summary{Healthy: true})
	klog.V(2).Info("Starting report retriever")

	for {
		// always wait for new uploaded archive or insights-operator ends
		select {
		case <-r.archiveUploadReporter:
			// When a new archive is uploaded, try to get the report and repeat every 30s
			// until the report is retrieved
			r.insightsReport = make(chan struct{})
			wait.Until(r.PullSmartProxy, time.Duration(30e9), r.insightsReport)
		case <-ctx.Done():
			return
		}
	}
}

func updateInsightsMetrics(report types.SmartProxyReport) {
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
}
