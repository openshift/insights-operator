package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func Test_deleteAllRunningGatheringsPods(t *testing.T) {
	tests := []struct {
		name            string
		existingJobs    []batchv1.Job
		expectedDeletes []string
	}{
		{
			name: "active gathering job is deleted",
			existingJobs: []batchv1.Job{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "periodic-gathering-12345",
						Namespace: "openshift-insights",
					},
					Status: batchv1.JobStatus{
						Active: 1,
					},
				},
			},
			expectedDeletes: []string{"periodic-gathering-12345"},
		},
		{
			name: "only active jobs are deleted, not completed",
			existingJobs: []batchv1.Job{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "periodic-gathering-active1",
						Namespace: "openshift-insights",
					},
					Status: batchv1.JobStatus{
						Active: 1,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "periodic-gathering-completed",
						Namespace: "openshift-insights",
					},
					Status: batchv1.JobStatus{
						Active:    0,
						Succeeded: 1,
					},
				},
			},
			expectedDeletes: []string{"periodic-gathering-active1"},
		},
		{
			name: "only periodic-gathering prefix jobs are deleted",
			existingJobs: []batchv1.Job{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "periodic-gathering-12345",
						Namespace: "openshift-insights",
					},
					Status: batchv1.JobStatus{
						Active: 1,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-job-67890",
						Namespace: "openshift-insights",
					},
					Status: batchv1.JobStatus{
						Active: 1,
					},
				},
			},
			expectedDeletes: []string{"periodic-gathering-12345"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create fake clientset with existing jobs
			var objs []runtime.Object
			for i := range tt.existingJobs {
				objs = append(objs, &tt.existingJobs[i])
			}
			fakeClient := fake.NewSimpleClientset(objs...)

			// Call the function
			deleteAllRunningGatheringsPods(ctx, fakeClient)

			// Verify the expected jobs were deleted
			remainingJobs, err := fakeClient.BatchV1().Jobs("openshift-insights").List(ctx, metav1.ListOptions{})
			assert.NoError(t, err)

			// Calculate which jobs should remain
			expectedRemainingCount := len(tt.existingJobs) - len(tt.expectedDeletes)
			assert.Equal(t, expectedRemainingCount, len(remainingJobs.Items), "Number of remaining jobs should match")

			// Verify that the jobs we expected to delete are not in the remaining list
			remainingJobNames := make(map[string]bool)
			for _, job := range remainingJobs.Items {
				remainingJobNames[job.Name] = true
			}

			for _, deletedJobName := range tt.expectedDeletes {
				assert.False(t, remainingJobNames[deletedJobName])
			}
		})
	}
}
