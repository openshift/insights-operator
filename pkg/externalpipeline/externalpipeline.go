package externalpipeline

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/RedHatInsights/insights-results-aggregator/types"
	"k8s.io/client-go/pkg/version"
	"k8s.io/klog"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/openshift/insights-operator/pkg/config"
)

type ReportsCache struct {
	Configuration config.SmartProxy
	Report        types.ClusterReport
	client        *http.Client
	authorizer    Authorizer
	clusterInfo   ClusterVersionInfo
}

type ClusterVersionInfo interface {
	ClusterVersion() *configv1.ClusterVersion
}

type Authorizer interface {
	Authorize(req *http.Request) error
	NewSystemOrConfiguredProxy() func(*http.Request) (*url.URL, error)
}

var ErrWaitingForVersion = fmt.Errorf("waiting for the cluster version to be loaded")

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

		r.Report = types.ClusterReport(body)
		klog.Info("Insights report retrieved")
		klog.Info(r.Report)
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
