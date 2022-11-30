package insightsuploader

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/controllerstatus"
	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
)

type Uploader interface {
	//TODO
}

type Controller struct {
	controllerstatus.StatusController

	client             *insightsclient.Client
	secretConfigurator configobserver.Configurator
	apiConfigurator    configobserver.APIConfigObserver
	bo                 wait.Backoff
}

func New(client *insightsclient.Client,
	secretconfigurator configobserver.Configurator,
	apiConfigurator configobserver.APIConfigObserver) *Controller {
	controller := &Controller{
		secretConfigurator: secretconfigurator,
		apiConfigurator:    apiConfigurator,
		client:             client,
	}
	controller.bo = wait.Backoff{
		Duration: controller.secretConfigurator.Config().Interval / 4, // 30 min as first wait by default
		Steps:    4,
		Factor:   2,
	}
	return controller
}

func (c *Controller) Upload(ctx context.Context, s *insightsclient.Source) error {
	defer s.Contents.Close()
	start := time.Now()
	s.ID = start.Format(time.RFC3339)
	s.Type = "application/vnd.redhat.openshift.periodic"
	err := wait.ExponentialBackoff(c.bo, func() (done bool, err error) {
		err = c.client.Send(ctx, c.secretConfigurator.Config().Endpoint, *s)
		if err != nil {
			klog.V(2).Infof("Unable to upload report after %s: %v", time.Since(start).Truncate(time.Second/100), err)
			klog.Errorf("%v. Trying again in %s %d", err, c.bo.Step(), c.bo.Steps)
			return false, nil
			// TODO we would need to propagate the error as HTTP
		}
		return true, nil
	})
	if err != nil {
		return err
	}
	klog.Infof("Uploaded report successfully in %s", time.Since(start))
	return nil
}

/* func (c *Controller) Run(ctx context.Context) {
	c.StatusController.UpdateStatus(controllerstatus.Summary{Healthy: true})

	if c.client == nil {
		klog.Infof("No reporting possible without a configured client")
		return
	}

	// the controller periodically uploads results to the remote insights endpoint
	cfg := c.secretConfigurator.Config()
	configCh, cancelFn := c.secretConfigurator.ConfigChanged()
	defer cancelFn()

	reportingEnabled := cfg.Report
	endpoint := cfg.Endpoint
	interval := cfg.Interval
	lastReported := c.reporter.LastReportedTime()
	if !lastReported.IsZero() {
		next := lastReported.Add(interval)
		if now := time.Now(); next.After(now) {
			c.initialDelay = wait.Jitter(now.Sub(next), 1.2)
		}
	}
	klog.V(2).Infof("Reporting status periodically to %s every %s, starting in %s", cfg.Endpoint, interval, c.initialDelay.Truncate(time.Second))

	wait.Until(func() {
		if c.initialDelay > 0 {
			select {
			case <-ctx.Done():
			case <-time.After(c.initialDelay):
			case <-configCh:
				newCfg := c.secretConfigurator.Config()
				interval = newCfg.Interval
				endpoint = newCfg.Endpoint
				reportingEnabled = newCfg.Report
				var disabledInAPI bool
				if c.apiConfigurator != nil {
					disabledInAPI = c.apiConfigurator.GatherDisabled()
				}
				if !reportingEnabled || disabledInAPI {
					klog.V(2).Infof("Reporting was disabled")
					c.initialDelay = newCfg.Interval
					return
				}
			}
			c.initialDelay = 0
		}

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

		c.initialDelay = wait.Jitter(interval, 1.2)
	}, 15*time.Second, ctx.Done())
}

// ArchiveUploaded returns a channel that indicates when an archive is uploaded
func (c *Controller) ArchiveUploaded() <-chan struct{} {
	return c.archiveUploaded
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
*/
