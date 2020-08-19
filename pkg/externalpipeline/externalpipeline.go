package externalpipeline

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/RedHatInsights/insights-results-smart-proxy/types"
	"k8s.io/client-go/pkg/version"
	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/klog"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/openshift/insights-operator/pkg/config"
)

type ReportsCache struct {
	Configuration config.SmartProxy
	client        *http.Client
	authorizer    Authorizer
	clusterInfo   ClusterVersionInfo
	LastReport    types.SmartProxyReport
}

type InsightsReportResponse struct {
	Report types.SmartProxyReport `json:"report"`
	Status string                 `json:"status"`
}

type ClusterVersionInfo interface {
	ClusterVersion() *configv1.ClusterVersion
}

type Authorizer interface {
	Authorize(req *http.Request) error
	NewSystemOrConfiguredProxy() func(*http.Request) (*url.URL, error)
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

func New(c config.SmartProxy, authorizer Authorizer, clusterInfo ClusterVersionInfo) *ReportsCache {
	return &ReportsCache{
		Configuration: c,
		client:        &http.Client{},
		authorizer:    authorizer,
		clusterInfo:   clusterInfo,
	}
}

func (r *ReportsCache) PullSmartProxy() error {
	klog.Info("Pulling report from smart-proxy")
	cv := r.clusterInfo.ClusterVersion()
	if cv == nil {
		klog.Warning(ErrWaitingForVersion)
		return ErrWaitingForVersion
	}

	endpoint := fmt.Sprintf(r.Configuration.Endpoint, cv.Spec.ClusterID)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)

	klog.Infof("Retrieving report for cluster: %s", cv.Spec.ClusterID)
	klog.Infof("Endpoint: %s", endpoint)
	if err != nil {
		klog.Error(err)
		return err
	}

	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header.Set("User-Agent", fmt.Sprintf("insights-operator/%s cluster/%s", version.Get().GitCommit, cv.Spec.ClusterID))

	if err := r.authorizer.Authorize(req); err != nil {
		klog.Error(err)
		return err
	}

	klog.Infof("Headers: %v\n", req.Header)

	response, err := r.client.Do(req)
	if err != nil {
		klog.Errorf("Unable to retrieve latest report for %s: %v", cv.Spec.ClusterID, err)
		return err
	}

	if response.StatusCode == http.StatusOK {
		body, err := ioutil.ReadAll(response.Body)

		if err != nil {
			klog.Error("The response from Smart Proxy cannot be read")
			return err
		}

		reportResponse := InsightsReportResponse{}
		if err = json.Unmarshal(body, &reportResponse); err != nil {
			klog.Error("The report response cannot be parsed")
		} else {
			updateInsightsMetrics(reportResponse.Report)
			r.LastReport = reportResponse.Report
		}
	} else {
		klog.Warningf("Report response status code: %d", response.StatusCode)
	}

	return nil
}

func (r ReportsCache) Run() {
	klog.Info("Starting report retriever")
	uptimeTicker := time.NewTicker(r.Configuration.PollTime)

	for {
		r.PullSmartProxy()
		select {
		case <-uptimeTicker.C:
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
	if err := legacyregistry.Register(
		insightsStatus,
	); err != nil {
		fmt.Println(err)
	}
}
