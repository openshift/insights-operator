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

type ReportsCache struct {
	controllerstatus.Simple
	Configuration config.SmartProxy
	client        *insightsclient.Client
	LastReport    types.SmartProxyReport
}

type InsightsReportResponse struct {
	Report types.SmartProxyReport `json:"report"`
	Status string                 `json:"status"`
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

func New(c config.SmartProxy, client *insightsclient.Client) *ReportsCache {
	return &ReportsCache{
		Simple:        controllerstatus.Simple{Name: "insightsreport"},
		Configuration: c,
		client:        client,
	}
}

func (r *ReportsCache) PullSmartProxy() {
	klog.Info("Pulling report from smart-proxy")
	ctx, cancelFunc := context.WithTimeout(context.Background(), time.Minute)
	defer cancelFunc()

	klog.Info("Retrieving report")
	reportBody, err := r.client.RecvReport(ctx, r.Configuration.Endpoint)
	klog.Info("Report retrieved")
	if authorizer.IsAuthorizationError(err) {
		r.Simple.UpdateStatus(controllerstatus.Summary{
			Operation: controllerstatus.DownloadingReport,
			Reason:    "NotAuthorized",
			Message:   fmt.Sprintf("Auth rejected for downloading latest report: %v", err),
		})
		return
	} else if err != nil {
		klog.Error(err)
		return
	}

	klog.Info("Parsing report")
	reportResponse := InsightsReportResponse{}

	if err = json.NewDecoder(*reportBody).Decode(&reportResponse); err != nil {
		klog.Error("The report response cannot be parsed")
		return
	}

	updateInsightsMetrics(reportResponse.Report)
	r.LastReport = reportResponse.Report
	return
}

func (r ReportsCache) Run(ctx context.Context) {
	r.Simple.UpdateStatus(controllerstatus.Summary{Healthy: true})
	klog.Info("Starting report retriever")
	wait.Until(r.PullSmartProxy, r.Configuration.PollTime, ctx.Done())
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
