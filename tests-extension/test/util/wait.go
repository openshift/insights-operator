package util

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	insightsv1 "github.com/openshift/api/insights/v1"
	insightsclientset "github.com/openshift/client-go/insights/clientset/versioned"
)

// dataRecordedConditionType is the DataGather status condition that reports
// whether the archive was written, independent of upload/processing.
const dataRecordedConditionType = "DataRecorded"

// WaitForDataGatherRecorded polls the named DataGather until its DataRecorded
// condition is True (returning the final DataGather), False (returning an
// error built from the condition's message immediately), or the timeout
// elapses (returning an error dumping the full condition list).
func WaitForDataGatherRecorded(
	ctx context.Context, insightsClient insightsclientset.Interface, name string, timeout time.Duration,
) (*insightsv1.DataGather, error) {
	var latest *insightsv1.DataGather

	pollErr := wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		dg, err := insightsClient.InsightsV1().DataGathers().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		latest = dg

		cond := meta.FindStatusCondition(dg.Status.Conditions, dataRecordedConditionType)
		if cond == nil {
			return false, nil
		}

		switch cond.Status {
		case metav1.ConditionTrue:
			return true, nil
		case metav1.ConditionFalse:
			return false, fmt.Errorf(
				"DataGather %s failed to record data (reason %s): %s", name, cond.Reason, cond.Message,
			)
		case metav1.ConditionUnknown:
			return false, nil
		}
		return false, nil
	})
	if pollErr != nil {
		if wait.Interrupted(pollErr) {
			return latest, fmt.Errorf(
				"timed out waiting for DataGather %s to record data, conditions: %#v", name, conditionsOf(latest),
			)
		}
		return latest, pollErr
	}

	return latest, nil
}

func conditionsOf(dg *insightsv1.DataGather) []metav1.Condition {
	if dg == nil {
		return nil
	}
	return dg.Status.Conditions
}

// WaitForGatheringJobTerminal polls the batch Job the operator creates for a
// DataGather.
//
// The Job itself is created asynchronously by the operator (an informer
// reacts to the DataGather and only then creates the Job), so a NotFound on
// an early poll is expected, not fatal: it means the Job hasn't been created
// yet, and polling should keep retrying rather than abort.
func WaitForGatheringJobTerminal(
	ctx context.Context, kubeClient kubernetes.Interface, name string, timeout time.Duration,
) (*batchv1.Job, error) {
	var latest *batchv1.Job

	pollErr := wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		job, err := kubeClient.BatchV1().Jobs(InsightsNamespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		latest = job

		for _, cond := range job.Status.Conditions {
			if cond.Status != corev1.ConditionTrue {
				continue
			}
			switch cond.Type {
			case batchv1.JobComplete, batchv1.JobFailed:
				return true, nil
			}
		}
		return false, nil
	})
	if pollErr != nil {
		if wait.Interrupted(pollErr) {
			return latest, fmt.Errorf("timed out waiting for gathering job %s/%s to reach a terminal state",
				InsightsNamespace, name)
		}
		return latest, pollErr
	}

	return latest, nil
}

// DeleteGatheringJob deletes the gathering Job with Foreground propagation,
// ignoring a not-found error.
func DeleteGatheringJob(ctx context.Context, kubeClient kubernetes.Interface, name string) error {
	propagation := metav1.DeletePropagationForeground
	err := kubeClient.BatchV1().Jobs(InsightsNamespace).Delete(ctx, name, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete gathering job %s/%s: %w", InsightsNamespace, name, err)
	}
	return nil
}
