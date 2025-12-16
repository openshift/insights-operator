package periodic

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_isJobComplete(t *testing.T) {
	tests := []struct {
		name       string
		job        *batchv1.Job
		wantResult bool
	}{
		{
			name: "Job with Complete condition set to True",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "Job with Complete condition set to False",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			wantResult: false,
		},
		{
			name: "Job with no conditions",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{},
				},
			},
			wantResult: false,
		},
		{
			name: "Job with only Failed condition",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobFailed,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isJobComplete(tt.job)
			assert.Equal(t, tt.wantResult, result)
		})
	}
}

func Test_isJobFailed(t *testing.T) {
	tests := []struct {
		name       string
		job        *batchv1.Job
		wantResult bool
	}{
		{
			name: "Job with Failed condition set to True",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobFailed,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "Job with Failed condition set to False",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobFailed,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			wantResult: false,
		},
		{
			name: "Job with no conditions",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{},
				},
			},
			wantResult: false,
		},
		{
			name: "Job with only Complete condition",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isJobFailed(tt.job)
			assert.Equal(t, tt.wantResult, result)
		})
	}
}

func Test_isJobFinished(t *testing.T) {
	tests := []struct {
		name       string
		job        *batchv1.Job
		wantResult bool
	}{
		{
			name: "Job with Complete condition",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "Job with Failed condition",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobFailed,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "Job with both Complete and Failed conditions",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   batchv1.JobFailed,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "Job with no conditions",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{},
				},
			},
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isJobFinished(tt.job)
			assert.Equal(t, tt.wantResult, result)
		})
	}
}

func Test_eventHandler_UpdateFunc(t *testing.T) {
	tests := []struct {
		name           string
		oldJob         *batchv1.Job
		newJob         *batchv1.Job
		shouldSendName bool
		expectedName   string
	}{
		{
			name: "Job transitions from running to failed should send job name",
			oldJob: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "failed-job"},
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{},
				},
			},
			newJob: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "failed-job"},
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{Type: batchv1.JobFailed, Status: corev1.ConditionTrue},
					},
				},
			},
			shouldSendName: true,
			expectedName:   "failed-job",
		},
		{
			name: "Job transitions from running to complete should not send job name",
			oldJob: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "test-job"},
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{},
				},
			},
			newJob: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "test-job"},
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
					},
				},
			},
			shouldSendName: false,
			expectedName:   "test-job",
		},
		{
			name: "Job already completed, no change",
			oldJob: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "old-complete"},
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
					},
				},
			},
			newJob: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "old-complete"},
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
					},
				},
			},
			shouldSendName: false,
		},
		{
			name: "Job still running",
			oldJob: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "running"},
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{},
				},
			},
			newJob: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "running"},
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{},
				},
			},
			shouldSendName: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			watcher := &JobCompletionWatcher{
				finishedJobChannel: make(chan string, 1),
			}

			// Get the event handler
			handler := watcher.eventHandler()

			// Trigger update event
			handler.OnUpdate(tt.oldJob, tt.newJob)

			// Check if we received the expected job name
			if tt.shouldSendName {
				select {
				case jobName := <-watcher.finishedJobChannel:
					assert.Equal(t, tt.expectedName, jobName)
				case <-time.After(100 * time.Millisecond):
					t.Fatal("Expected job name on channel but got nothing")
				}
			} else {
				select {
				case jobName := <-watcher.finishedJobChannel:
					t.Fatalf("Did not expect job name on channel but got: %s", jobName)
				case <-time.After(50 * time.Millisecond):
					// Expected - no job name sent
				}
			}
		})
	}
}

func Test_eventHandler_DeleteFunc(t *testing.T) {
	tests := []struct {
		name           string
		job            interface{}
		shouldSendName bool
		expectedName   string
	}{
		{
			name: "Job deleted should send job name to channel",
			job: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "deleted-job"},
			},
			shouldSendName: true,
			expectedName:   "deleted-job",
		},
		{
			name:           "Wrong type should not send job name to channel",
			job:            &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "not-a-job"}},
			shouldSendName: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			watcher := &JobCompletionWatcher{
				deletedJobChannel: make(chan string, 1),
			}

			// Get the event handler
			handler := watcher.eventHandler()

			// Trigger delete event
			handler.OnDelete(tt.job)

			// Check if we received the expected job name
			if tt.shouldSendName {
				select {
				case jobName := <-watcher.deletedJobChannel:
					assert.Equal(t, tt.expectedName, jobName)
				case <-time.After(100 * time.Millisecond):
					t.Fatal("Expected job name on channel but got nothing")
				}
			} else {
				select {
				case jobName := <-watcher.deletedJobChannel:
					t.Fatalf("Did not expect job name on channel but got: %s", jobName)
				case <-time.After(50 * time.Millisecond):
					// Expected - no job name sent
				}
			}
		})
	}
}
