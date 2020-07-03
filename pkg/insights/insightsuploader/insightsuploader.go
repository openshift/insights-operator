package insightsuploader

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"time"

	"k8s.io/klog"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/openshift/insights-operator/pkg/authorizer"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/controllerstatus"
	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
)

type Configurator interface {
	Config() *config.Controller
	ConfigChanged() (<-chan struct{}, func())
}

type Authorizer interface {
	IsAuthorizationError(error) bool
}

type Summarizer interface {
	Summary(ctx context.Context, since time.Time) (io.ReadCloser, bool, error)
}

type StatusReporter interface {
	LastReportedTime() time.Time
	SetLastReportedTime(time.Time)
	SafeInitialStart() bool
	SetSafeInitialStart(s bool)
}

type Controller struct {
	controllerstatus.Simple

	summarizer   Summarizer
	client       *insightsclient.Client
	configurator Configurator
	reporter     StatusReporter
}

func New(summarizer Summarizer, client *insightsclient.Client, configurator Configurator, statusReporter StatusReporter) *Controller {
	return &Controller{
		Simple: controllerstatus.Simple{Name: "insightsuploader"},

		summarizer:   summarizer,
		configurator: configurator,
		client:       client,
		reporter:     statusReporter,
	}
}

func (c *Controller) Run(ctx context.Context) {
	c.Simple.UpdateStatus(controllerstatus.Summary{Healthy: true})

	if c.client == nil {
		klog.Infof("No reporting possible without a configured client")
		return
	}

	// the controller periodically uploads results to the remote insights endpoint
	cfg := c.configurator.Config()
	configCh, cancelFn := c.configurator.ConfigChanged()
	defer cancelFn()

	enabled := cfg.Report
	endpoint := cfg.Endpoint
	interval := cfg.Interval
	initialDelay := wait.Jitter(interval/8, 2)
	lastReported := c.reporter.LastReportedTime()
	if !lastReported.IsZero() {
		next := lastReported.Add(interval)
		if now := time.Now(); next.After(now) {
			initialDelay = wait.Jitter(now.Sub(next), 1.2)
		}
	}
	if c.reporter.SafeInitialStart() {
		initialDelay = 0
	}
	klog.V(2).Infof("Reporting status periodically to %s every %s, starting in %s", cfg.Endpoint, interval, initialDelay.Truncate(time.Second))

	wait.Until(func() {
		if initialDelay > 0 {
			select {
			case <-ctx.Done():
			case <-time.After(initialDelay):
			case <-configCh:
				newCfg := c.configurator.Config()
				interval = newCfg.Interval
				endpoint = newCfg.Endpoint
				if newCfg.Report != enabled {
					enabled = newCfg.Report
					if !newCfg.Report {
						klog.V(2).Infof("Reporting was disabled")
						initialDelay = newCfg.Interval
						return
					}
					klog.V(2).Infof("Reporting was enabled")
				}
			}
			initialDelay = 0
		}

		// attempt to get a summary to send to the server
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		source, ok, err := c.summarizer.Summary(ctx, lastReported)
		if err != nil {
			c.Simple.UpdateStatus(controllerstatus.Summary{Reason: "SummaryFailed", Message: fmt.Sprintf("Unable to retrieve local insights data: %v", err)})
			return
		}
		if !ok {
			klog.V(4).Infof("Nothing to report since %s", lastReported.Format(time.RFC3339))
			return
		}
		defer source.Close()

		if enabled && len(endpoint) > 0 {
			// send the results
			start := time.Now()
			id := start.Format(time.RFC3339)
			klog.V(4).Infof("Uploading latest report since %s", lastReported.Format(time.RFC3339))
			if err := c.client.Send(ctx, endpoint, insightsclient.Source{
				ID:       id,
				Type:     "application/vnd.redhat.openshift.periodic",
				Contents: source,
			}); err != nil {
				klog.V(2).Infof("Unable to upload report after %s: %v", time.Now().Sub(start).Truncate(time.Second/100), err)
				if err == insightsclient.ErrWaitingForVersion {
					initialDelay = wait.Jitter(interval/8, 1) - interval/8
					if c.reporter.SafeInitialStart() {
						initialDelay = wait.Jitter(time.Second*15, 1)
					}
					return
				}
				c.reporter.SetSafeInitialStart(false)
				if authorizer.IsAuthorizationError(err) {
					c.Simple.UpdateStatus(controllerstatus.Summary{Operation: controllerstatus.Uploading,
						Reason: "NotAuthorized", Message: fmt.Sprintf("Reporting was not allowed: %v", err)})
					initialDelay = wait.Jitter(interval, 3)
					return
				}

				initialDelay = wait.Jitter(interval/8, 1.2)
				c.Simple.UpdateStatus(controllerstatus.Summary{Operation: controllerstatus.Uploading,
					Reason: "UploadFailed", Message: fmt.Sprintf("Unable to report: %v", err)})
				return
			}
			c.reporter.SetSafeInitialStart(false)
			klog.V(4).Infof("Uploaded report successfully in %s", time.Now().Sub(start))
			lastReported = start.UTC()
			c.Simple.UpdateStatus(controllerstatus.Summary{Healthy: true})
		} else {
			klog.V(4).Info("Display report that would be sent")
			// display what would have been sent (to ensure we always exercise source processing)
			if err := reportToLogs(source, klog.V(4)); err != nil {
				klog.Errorf("Unable to log upload: %v", err)
			}
			// we didn't actually report logs, so don't advance the report date
		}

		c.reporter.SetLastReportedTime(lastReported)

		initialDelay = wait.Jitter(interval, 1.2)
	}, 15*time.Second, ctx.Done())
}

func reportToLogs(source io.Reader, klog klog.Verbose) error {
	if !klog {
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
