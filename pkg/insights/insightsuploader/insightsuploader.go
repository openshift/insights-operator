package insightsuploader

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	configv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/openshift/insights-operator/pkg/authorizer"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/controller/status"
	"github.com/openshift/insights-operator/pkg/controllerstatus"
	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Authorizer interface {
	IsAuthorizationError(error) bool
}

type Summarizer interface {
	Summary(ctx context.Context, since time.Time) (*insightsclient.Source, bool, error)
}

type StatusReporter interface {
	LastReportedTime() time.Time
	SetLastReportedTime(time.Time)
}

type Controller struct {
	controllerstatus.StatusController

	summarizer      Summarizer
	client          *insightsclient.Client
	configurator    configobserver.Interface
	apiConfigurator configobserver.InsightsDataGatherObserver
	reporter        StatusReporter
	archiveUploaded chan struct{}
	uploadDelay     time.Duration
	backoff         wait.Backoff
}

func New(summarizer Summarizer,
	client *insightsclient.Client,
	configurator configobserver.Interface,
	apiConfigurator configobserver.InsightsDataGatherObserver,
	statusReporter StatusReporter,
	initialDelay time.Duration) *Controller {

	ctrl := &Controller{
		StatusController: controllerstatus.New("insightsuploader"),
		summarizer:       summarizer,
		configurator:     configurator,
		apiConfigurator:  apiConfigurator,
		client:           client,
		reporter:         statusReporter,
		archiveUploaded:  make(chan struct{}),
		uploadDelay:      initialDelay,
	}
	ctrl.backoff = wait.Backoff{
		Duration: ctrl.configurator.Config().DataReporting.Interval / 4, // 30 min as first wait by default
		Steps:    4,
		Factor:   2,
	}
	return ctrl
}

func (c *Controller) Run(ctx context.Context, initialDelay time.Duration) {
	c.StatusController.UpdateStatus(controllerstatus.Summary{Healthy: true})

	if c.client == nil {
		klog.Infof("No reporting possible without a configured client")
		return
	}

	cfg := c.configurator.Config()
	interval := cfg.DataReporting.Interval
	// set the initial upload delay as initial delay + 2 minutes
	ud := 90 * time.Second
	c.uploadDelay = time.Duration(initialDelay.Nanoseconds() + ud.Nanoseconds())

	klog.Infof("Reporting status periodically to %s every %s, starting in %s", cfg.DataReporting.UploadEndpoint, interval, c.uploadDelay.Truncate(time.Second))
	go wait.Until(func() { c.periodicTrigger(ctx.Done()) }, 5*time.Second, ctx.Done())
}

func (c *Controller) periodicTrigger(stopCh <-chan struct{}) {
	cfg := c.configurator.Config()
	interval := cfg.DataReporting.Interval
	var disabledInAPI bool
	if c.apiConfigurator != nil {
		disabledInAPI = c.apiConfigurator.GatherDisabled()
	}
	reportingEnabled := cfg.DataReporting.Enabled && !disabledInAPI
	configCh, cancelFn := c.configurator.ConfigChanged()
	defer cancelFn()

	ticker := time.NewTicker(c.uploadDelay)
	for {
		select {
		case <-stopCh:
			ticker.Stop()
		case <-ticker.C:
			c.checkSummaryAndSend(reportingEnabled)
			ticker.Reset(c.uploadDelay)
			return
		case <-configCh:
			newCfg := c.configurator.Config()
			reportingEnabled = newCfg.DataReporting.Enabled
			var disabledInAPI bool
			if c.apiConfigurator != nil {
				disabledInAPI = c.apiConfigurator.GatherDisabled()
			}
			if !reportingEnabled || disabledInAPI {
				klog.Infof("Reporting was disabled")
			}
			newInterval := newCfg.DataReporting.Interval
			if newInterval != interval {
				c.uploadDelay = wait.Jitter(interval/8, 0.1)
				ticker.Reset(c.uploadDelay)
				return
			}
		}
	}
}

func (c *Controller) checkSummaryAndSend(reportingEnabled bool) {
	lastReported := c.reporter.LastReportedTime()
	endpoint := c.configurator.Config().DataReporting.UploadEndpoint
	interval := c.configurator.Config().DataReporting.Interval
	c.uploadDelay = wait.Jitter(interval/8, 0.1)

	// attempt to get a summary to send to the server
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	source, ok, err := c.summarizer.Summary(ctx, lastReported)
	if err != nil {
		c.StatusController.UpdateStatus(controllerstatus.Summary{Reason: "SummaryFailed", Message: fmt.Sprintf("Unable to retrieve local insights data: %v", err)})
		return
	}
	if !ok {
		klog.Infof("Nothing to report since %s", lastReported.Format(time.RFC3339))
		return
	}

	klog.Infof("Checking archives to upload periodically every %s", c.uploadDelay)
	defer source.Contents.Close()

	if !reportingEnabled || len(endpoint) == 0 {
		klog.Info("Display report that would be sent")
		// display what would have been sent (to ensure we always exercise source processing)
		if err := reportToLogs(source.Contents); err != nil {
			klog.Errorf("Unable to log upload: %v", err)
		}
		return
	}

	// send the results
	start := time.Now()
	id := start.Format(time.RFC3339)
	klog.Infof("Uploading latest report since %s", lastReported.Format(time.RFC3339))
	source.ID = id
	source.Type = "application/vnd.redhat.openshift.periodic"
	if err := c.client.Send(ctx, endpoint, *source); err != nil {
		klog.Infof("Unable to upload report after %s: %v", time.Since(start).Truncate(time.Second/100), err)
		if errors.Is(err, insightsclient.ErrWaitingForVersion) {
			c.uploadDelay = wait.Jitter(time.Second*15, 1)
			return
		}
		if authorizer.IsAuthorizationError(err) {
			c.StatusController.UpdateStatus(controllerstatus.Summary{Operation: controllerstatus.Uploading,
				Reason: "NotAuthorized", Message: fmt.Sprintf("Reporting was not allowed: %v", err)})
			c.uploadDelay = wait.Jitter(interval/2, 2)

			return
		}

		c.uploadDelay = wait.Jitter(interval/8, 1.2)
		c.StatusController.UpdateStatus(controllerstatus.Summary{Operation: controllerstatus.Uploading,
			Reason: "UploadFailed", Message: fmt.Sprintf("Unable to report: %v", err)})
		return
	}
	klog.Infof("Uploaded report successfully in %s", time.Since(start))
	select {
	case c.archiveUploaded <- struct{}{}:
	default:
	}
	lastReported = start.UTC()
	c.StatusController.UpdateStatus(controllerstatus.Summary{Healthy: true})
	c.reporter.SetLastReportedTime(lastReported)
}

// ArchiveUploaded returns a channel that indicates when an archive is uploaded
func (c *Controller) ArchiveUploaded() <-chan struct{} {
	return c.archiveUploaded
}

// Upload is an alternative simple upload method used only in TechPreview clusters.
// Returns Insights request ID and error=nil in case of successful data upload.
func (c *Controller) Upload(ctx context.Context, s *insightsclient.Source, configClient configv1.ConfigV1Interface) (string, int, error) {
	defer s.Contents.Close()
	start := time.Now()
	s.ID = start.Format(time.RFC3339)
	s.Type = "application/vnd.redhat.openshift.periodic"
	var requestID string
	var statusCode int
	err := wait.ExponentialBackoff(c.backoff, func() (done bool, err error) {
		requestID, statusCode, err = c.client.SendAndGetID(ctx, c.configurator.Config().DataReporting.UploadEndpoint, *s)
		if err != nil {
			// do no return the error if it's not the last attempt
			if c.backoff.Steps > 1 {
				klog.Infof("Unable to upload report after %s: %v", time.Since(start).Truncate(time.Second/100), err)
				klog.Errorf("%v. Trying again in %s", err, c.backoff.Step())
				return false, nil
			}
		}
		return true, err
	})
	if err != nil {
		return "", statusCode, err
	}

	klog.Infof("Uploaded report successfully in %s", time.Since(start))

	// Update LastReportTime after successful upload
	updateClusterOperatorLastReportTime(ctx, configClient)

	return requestID, statusCode, nil
}

func reportToLogs(source io.Reader) error {
	gr, err := gzip.NewReader(source)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		klog.Infof("Dry-run: %s %7d %s", hdr.ModTime.Format(time.RFC3339), hdr.Size, hdr.Name)
	}
	return nil
}

// Update the ClusterOperator's lastReportTime extension field
func updateClusterOperatorLastReportTime(ctx context.Context, client configv1.ConfigV1Interface) error {
	insightsCo, err := client.ClusterOperators().Get(ctx, "insights", metav1.GetOptions{})
	if err != nil {
		return err
	}

	reported := status.Reported{
		LastReportTime: metav1.Time{Time: time.Now().UTC()},
	}

	data, err := json.Marshal(reported)
	if err != nil {
		return fmt.Errorf("unable to marshal status extension: %v", err)
	}
	insightsCo.Status.Extension.Raw = data

	_, err = client.ClusterOperators().UpdateStatus(ctx, insightsCo, metav1.UpdateOptions{})

	if err != nil {
		klog.Errorf("Failed to update LastReportTime: %v", err)
	}

	klog.Infof("Successfully updated LastReportTime to %s", reported.LastReportTime)
	return nil
}
