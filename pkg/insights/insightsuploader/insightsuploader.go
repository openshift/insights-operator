package insightsuploader

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/authorizer"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/controllerstatus"
	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
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
	initialDelay    time.Duration
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
		initialDelay:     initialDelay,
	}
	ctrl.backoff = wait.Backoff{
		Duration: ctrl.configurator.Config().DataReporting.Interval / 4, // 30 min as first wait by default
		Steps:    4,
		Factor:   2,
	}
	return ctrl
}

func (c *Controller) Run(ctx context.Context) {
	c.StatusController.UpdateStatus(controllerstatus.Summary{Healthy: true})

	if c.client == nil {
		klog.Infof("No reporting possible without a configured client")
		return
	}

	// the controller periodically uploads results to the remote insights endpoint
	cfg := c.configurator.Config()

	interval := cfg.DataReporting.Interval
	lastReported := c.reporter.LastReportedTime()
	if !lastReported.IsZero() {
		next := lastReported.Add(interval)
		if now := time.Now(); next.After(now) {
			c.initialDelay = wait.Jitter(next.Sub(now), 1.2)
		}
	}
	klog.V(2).Infof("Reporting status periodically to %s every %s, starting in %s", cfg.DataReporting.UploadEndpoint, interval, c.initialDelay.Truncate(time.Second))
	go wait.Until(func() { c.periodicTrigger(ctx.Done()) }, 5*time.Second, ctx.Done())
}

func (c *Controller) periodicTrigger(stopCh <-chan struct{}) {
	klog.Infof("Checking archives to upload periodically every %s", c.initialDelay)
	lastReported := c.reporter.LastReportedTime()
	cfg := c.configurator.Config()
	interval := cfg.DataReporting.Interval
	endpoint := cfg.DataReporting.UploadEndpoint
	var disabledInAPI bool
	if c.apiConfigurator != nil {
		disabledInAPI = c.apiConfigurator.GatherDisabled()
	}
	reportingEnabled := cfg.DataReporting.Enabled && !disabledInAPI

	configCh, cancelFn := c.configurator.ConfigChanged()
	defer cancelFn()

	if c.initialDelay <= 0 {
		c.checkSummaryAndSend(interval, lastReported, endpoint, reportingEnabled)
		return
	}
	ticker := time.NewTicker(c.initialDelay)
	for {
		select {
		case <-stopCh:
			ticker.Stop()
		case <-ticker.C:
			c.checkSummaryAndSend(interval, lastReported, endpoint, reportingEnabled)
			ticker.Reset(c.initialDelay)
			return
		case <-configCh:
			newCfg := c.configurator.Config()
			endpoint = newCfg.DataReporting.UploadEndpoint
			reportingEnabled = newCfg.DataReporting.Enabled
			var disabledInAPI bool
			if c.apiConfigurator != nil {
				disabledInAPI = c.apiConfigurator.GatherDisabled()
			}
			if !reportingEnabled || disabledInAPI {
				klog.V(2).Infof("Reporting was disabled")
				c.initialDelay = newCfg.DataReporting.Interval
				return
			}
			newInterval := newCfg.DataReporting.Interval
			if newInterval == interval {
				continue
			}
			interval = newInterval
			// there's no return in this case so set the initial delay again
			c.initialDelay = wait.Jitter(interval/8, 0.1)
			ticker.Reset(c.initialDelay)
		}
	}
}

func (c *Controller) checkSummaryAndSend(interval time.Duration, lastReported time.Time, endpoint string, reportingEnabled bool) {
	// attempt to get a summary to send to the server
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	source, ok, err := c.summarizer.Summary(ctx, lastReported)
	if err != nil {
		c.StatusController.UpdateStatus(controllerstatus.Summary{Reason: "SummaryFailed", Message: fmt.Sprintf("Unable to retrieve local insights data: %v", err)})
		return
	}
	if !ok {
		klog.V(4).Infof("Nothing to report since %s", lastReported.Format(time.RFC3339))
		return
	}
	defer source.Contents.Close()
	if reportingEnabled && len(endpoint) > 0 {
		// send the results
		start := time.Now()
		id := start.Format(time.RFC3339)
		klog.V(4).Infof("Uploading latest report since %s", lastReported.Format(time.RFC3339))
		source.ID = id
		source.Type = "application/vnd.redhat.openshift.periodic"
		if err := c.client.Send(ctx, endpoint, *source); err != nil {
			klog.V(2).Infof("Unable to upload report after %s: %v", time.Since(start).Truncate(time.Second/100), err)
			if err == insightsclient.ErrWaitingForVersion {
				c.initialDelay = wait.Jitter(time.Second*15, 1)
				return
			}
			if authorizer.IsAuthorizationError(err) {
				c.StatusController.UpdateStatus(controllerstatus.Summary{Operation: controllerstatus.Uploading,
					Reason: "NotAuthorized", Message: fmt.Sprintf("Reporting was not allowed: %v", err)})
				c.initialDelay = wait.Jitter(interval/2, 2)

				return
			}

			c.initialDelay = wait.Jitter(interval/8, 1.2)
			c.StatusController.UpdateStatus(controllerstatus.Summary{Operation: controllerstatus.Uploading,
				Reason: "UploadFailed", Message: fmt.Sprintf("Unable to report: %v", err)})
			return
		}
		klog.V(4).Infof("Uploaded report successfully in %s", time.Since(start))
		select {
		case c.archiveUploaded <- struct{}{}:
		default:
		}
		lastReported = start.UTC()
		c.StatusController.UpdateStatus(controllerstatus.Summary{Healthy: true})
	} else {
		klog.V(4).Info("Display report that would be sent")
		// display what would have been sent (to ensure we always exercise source processing)
		if err := reportToLogs(source.Contents, klog.V(4)); err != nil {
			klog.Errorf("Unable to log upload: %v", err)
		}
		// we didn't actually report logs, so don't advance the report date
	}

	c.reporter.SetLastReportedTime(lastReported)
	c.initialDelay = wait.Jitter(interval/8, 0.1)
}

// ArchiveUploaded returns a channel that indicates when an archive is uploaded
func (c *Controller) ArchiveUploaded() <-chan struct{} {
	return c.archiveUploaded
}

// Upload is an alternative simple upload method used only in TechPreview clusters.
// Returns Insights request ID and error=nil in case of successful data upload.
func (c *Controller) Upload(ctx context.Context, s *insightsclient.Source) (string, int, error) {
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
				klog.V(2).Infof("Unable to upload report after %s: %v", time.Since(start).Truncate(time.Second/100), err)
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
	return requestID, statusCode, nil
}

func reportToLogs(source io.Reader, klog klog.Verbose) error {
	if !klog.Enabled() {
		return nil
	}
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
