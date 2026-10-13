package controllerstatus

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_StatusController_Basic(t *testing.T) {
	testCtrl := New("test-controller")
	assert.Equal(t, "test-controller", testCtrl.Name())
	summary, ready := testCtrl.CurrentStatus()
	assert.False(t, ready, "controller should not be ready")
	assert.False(t, summary.Healthy, "controller should not be healthy")
	assert.Equal(t, 0, summary.Count)
	assert.True(t, summary.LastTransitionTime.IsZero())

	updatedSummary := Summary{
		Operation:          Operation{Name: "testing", HTTPStatusCode: 200},
		Healthy:            true,
		LastTransitionTime: time.Now(),
		Reason:             "UpdatedOK1",
		Message:            "testing status controller",
	}

	testCtrl.UpdateStatus(updatedSummary)
	firstUpdatedSummary, ready := testCtrl.CurrentStatus()
	assert.True(t, ready, "controller should be ready after status updated")
	assert.True(t, firstUpdatedSummary.Healthy, "controller should be healthy after status updated")
	assert.Equal(t, 1, firstUpdatedSummary.Count)
	assert.Equal(t, OperationName("testing"), firstUpdatedSummary.Operation.Name)
	assert.False(t, firstUpdatedSummary.LastTransitionTime.IsZero())

	updatedSummary2 := Summary{
		Operation:          Operation{Name: "testing2", HTTPStatusCode: 200},
		Healthy:            true,
		LastTransitionTime: time.Now(),
		Reason:             "UpdatedOK1",
		Message:            "testing status controller",
	}
	time.Sleep(10 * time.Millisecond)
	testCtrl.UpdateStatus(updatedSummary2)
	secondUpdatedSummary, ready := testCtrl.CurrentStatus()
	assert.True(t, ready, "controller should be ready after status updated")
	assert.True(t, secondUpdatedSummary.Healthy, "controller should be healthy after status updated")
	assert.Equal(t, 2, secondUpdatedSummary.Count)
	// LastTransition time should not change and same for the operation name
	assert.Equal(t, OperationName("testing"), secondUpdatedSummary.Operation.Name)
	assert.Equal(t, firstUpdatedSummary.LastTransitionTime, secondUpdatedSummary.LastTransitionTime)
}

func Test_StatusController_ChangingStatus(t *testing.T) {
	tests := []struct {
		name           string
		ctrl           StatusController
		firstStatus    Summary
		secondStatus   Summary
		expectedStatus Summary
	}{
		{
			name: "From healthy status to unhealthy",
			ctrl: New("test-controller-2"),
			firstStatus: Summary{
				Operation:          Operation{Name: "testing", HTTPStatusCode: 200},
				Healthy:            true,
				LastTransitionTime: time.Now(),
				Reason:             "UpdatedOK1",
				Message:            "testing status",
			},
			secondStatus: Summary{
				Operation:          Operation{Name: "testing2", HTTPStatusCode: 403},
				Healthy:            false,
				LastTransitionTime: time.Now(),
				Reason:             "Failed",
				Message:            "Something went wrong",
			},
			expectedStatus: Summary{
				Operation: Operation{Name: "testing2", HTTPStatusCode: 403},
				Healthy:   false,
				Reason:    "Failed",
				Message:   "Something went wrong",
				Count:     1,
			},
		},
		{
			name: "From healthy status to healthy but different reason",
			ctrl: New("test-controller-3"),
			firstStatus: Summary{
				Operation:          Operation{Name: "testing1", HTTPStatusCode: 200},
				Healthy:            true,
				LastTransitionTime: time.Now(),
				Reason:             "UpdatedOK1",
				Message:            "testing status",
			},
			secondStatus: Summary{
				Operation:          Operation{Name: "testing2", HTTPStatusCode: 202},
				Healthy:            true,
				LastTransitionTime: time.Now(),
				Reason:             "UpdatedOK2",
				Message:            "testing status2",
			},
			expectedStatus: Summary{
				Operation: Operation{Name: "testing2", HTTPStatusCode: 202},
				Healthy:   true,
				Reason:    "UpdatedOK2",
				Message:   "testing status2",
				Count:     1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.ctrl.UpdateStatus(tt.firstStatus)
			tt.ctrl.UpdateStatus(tt.secondStatus)
			currentStatus, _ := tt.ctrl.CurrentStatus()
			assert.Equal(t, tt.expectedStatus.Operation, currentStatus.Operation)
			assert.Equal(t, tt.expectedStatus.Healthy, currentStatus.Healthy)
			assert.Equal(t, tt.expectedStatus.Reason, currentStatus.Reason)
			assert.Equal(t, tt.expectedStatus.Message, currentStatus.Message)
			assert.Equal(t, tt.secondStatus.LastTransitionTime, currentStatus.LastTransitionTime)
			assert.True(t, tt.firstStatus.LastTransitionTime.Before(currentStatus.LastTransitionTime))
		})
	}
}
