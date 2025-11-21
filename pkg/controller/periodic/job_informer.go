package periodic

import (
	"context"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

type JobWatcher interface {
	factory.Controller
	FinishedJob() <-chan string
}

type JobCompletionWatcher struct {
	factory.Controller
	ch chan string
}

func NewJobCompletionWatcher(
	eventRecorder events.Recorder,
	sharedInformers informers.SharedInformerFactory,
) (*JobCompletionWatcher, error) {
	jobInformer := sharedInformers.Batch().V1().Jobs().Informer()

	jic := &JobCompletionWatcher{
		ch: make(chan string),
	}

	_, err := jobInformer.AddEventHandler(
		jic.eventHandler(),
	)
	if err != nil {
		return nil, err
	}

	ctrl := factory.New().WithInformers(jobInformer).
		WithSync(jic.sync).
		ToController("JobInformer", eventRecorder)

	jic.Controller = ctrl

	return jic, nil
}

func (w *JobCompletionWatcher) sync(_ context.Context, _ factory.SyncContext) error {
	return nil
}

func (w *JobCompletionWatcher) eventHandler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldJob, ok := oldObj.(*batchv1.Job)
			if !ok {
				klog.Errorf("Expected *batchv1.Job, got %T", oldObj)
				return
			}

			newJob, ok := newObj.(*batchv1.Job)
			if !ok {
				klog.Errorf("Expected *batchv1.Job, got %T", newObj)
				return
			}

			if isJobFinished(oldJob) {
				return
			}

			if isJobFailed(newJob) {
				klog.Infof("Job failed: %s", newJob.Name)

				select {
				case w.ch <- newJob.Name:
				default:
					klog.Info("Job channel full")
				}
			}
		},
	}
}

func (w *JobCompletionWatcher) FinishedJob() <-chan string {
	return w.ch
}

func isJobComplete(job *batchv1.Job) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func isJobFailed(job *batchv1.Job) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func isJobFinished(job *batchv1.Job) bool {
	return isJobComplete(job) || isJobFailed(job)
}
