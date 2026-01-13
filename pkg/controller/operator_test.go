package controller

import (
	"context"
	"testing"

	insightsv1 "github.com/openshift/api/insights/v1"
	fakeinsightsclient "github.com/openshift/client-go/insights/clientset/versioned/fake"
	"github.com/openshift/insights-operator/pkg/controller/status"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func Test_deleteAllRunningGatheringsPods(t *testing.T) {
	tests := []struct {
		name                string
		dataGatherName      string
		existingJobs        []runtime.Object
		existingDataGathers []runtime.Object
		shouldDeleteJob     bool
	}{
		{
			name:                "No jobs in namespace",
			existingJobs:        []runtime.Object{},
			existingDataGathers: []runtime.Object{},
			shouldDeleteJob:     false,
		},
		{
			name:           "Active periodic gathering job should be deleted",
			dataGatherName: "periodic-gathering-1",
			existingJobs: []runtime.Object{
				&batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "periodic-gathering-1",
						Namespace: insightsNamespace,
					},
					Status: batchv1.JobStatus{
						Active: 1,
					},
				},
			},
			existingDataGathers: []runtime.Object{
				&insightsv1.DataGather{
					ObjectMeta: metav1.ObjectMeta{
						Name: "periodic-gathering-1",
					},
					Status: insightsv1.DataGatherStatus{
						Conditions: []metav1.Condition{
							status.ProgressingCondition(status.GatheringReason),
						},
					},
				},
			},
			shouldDeleteJob: true,
		},
		{
			name: "Completed periodic gathering job should not be deleted",
			existingJobs: []runtime.Object{
				&batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "periodic-gathering-2",
						Namespace: insightsNamespace,
					},
					Status: batchv1.JobStatus{
						Active:    0,
						Succeeded: 1,
					},
				},
			},
			existingDataGathers: []runtime.Object{},
			shouldDeleteJob:     false,
		},
		{
			name: "Active job without periodic-gathering prefix should not be deleted",
			existingJobs: []runtime.Object{
				&batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "on-demand-job",
						Namespace: insightsNamespace,
					},
					Status: batchv1.JobStatus{
						Active: 1,
					},
				},
			},
			existingDataGathers: []runtime.Object{},
			shouldDeleteJob:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeClient := fake.NewSimpleClientset(tt.existingJobs...)
			insightsClient := fakeinsightsclient.NewSimpleClientset(tt.existingDataGathers...)

			// Call the function
			ctx := context.Background()
			deleteAllRunningGatheringsPods(ctx, kubeClient, insightsClient.InsightsV1())

			// Fetch existing jobs after deleteAllRunningGatheringsPods function run
			remainingJobs, err := kubeClient.BatchV1().Jobs(insightsNamespace).List(ctx, metav1.ListOptions{})
			assert.NoError(t, err)

			if tt.shouldDeleteJob {
				assert.Len(t, remainingJobs.Items, 0, "Job should have been deleted")

				// Verify DataGather CR Progressing condition was updated
				if len(tt.existingDataGathers) > 0 {
					updatedDG, err := insightsClient.InsightsV1().DataGathers().Get(ctx, tt.dataGatherName, metav1.GetOptions{})
					assert.NoError(t, err)

					// Check that Progressing condition was updated to False with GatheringFailed reason
					foundProgressingCondition := false
					for _, condition := range updatedDG.Status.Conditions {
						if condition.Type == status.Progressing {
							foundProgressingCondition = true
							assert.Equal(t, metav1.ConditionFalse, condition.Status,
								"Progressing condition should be False after job deletion")
							assert.Equal(t, status.GatheringFailedReason, condition.Reason,
								"Progressing condition reason should be GatheringFailed")
							break
						}
					}
					assert.True(t, foundProgressingCondition, "Progressing condition should exist in DataGather status")
				}
			} else {
				assert.Len(t, remainingJobs.Items, len(tt.existingJobs), "Job should not have been deleted")
			}
		})
	}
}
