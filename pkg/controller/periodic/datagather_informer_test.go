package periodic

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/api/insights/v1alpha2"
)

func Test_DataGatherController_EventHandler_AddFunc(t *testing.T) {
	tests := []struct {
		name                string
		dataGatherName      string
		shouldBeFiltered    bool
		expectedReceiveName bool
	}{
		{
			name:                "DataGather with periodic-gathering- prefix is filtered",
			dataGatherName:      "periodic-gathering-xyz123",
			shouldBeFiltered:    true,
			expectedReceiveName: false,
		},
		{
			name:                "DataGather without periodic prefix is not filtered",
			dataGatherName:      "my-custom-datagather",
			shouldBeFiltered:    false,
			expectedReceiveName: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dgCtrl := &dataGatherController{
				ch: make(chan string, 1),
			}

			handler := dgCtrl.eventHandler()

			dg := &v1alpha2.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.dataGatherName,
				},
			}

			// Trigger add event
			handler.OnAdd(dg, false)

			select {
			case name := <-dgCtrl.ch:
				assert.True(t, tt.expectedReceiveName, "Expected not to receive name but got: %s", name)
				assert.Equal(t, tt.dataGatherName, name)
			case <-time.After(100 * time.Millisecond):
				assert.False(t, tt.expectedReceiveName, "Expected to receive name but timed out")
			}
		})
	}
}

func Test_DataGatherController_EventHandler_InvalidObject(t *testing.T) {
	dgCtrl := &dataGatherController{
		ch: make(chan string, 1),
	}

	handler := dgCtrl.eventHandler()

	// Pass an invalid object (not a DataGather)
	invalidObj := "not a datagather object"
	handler.OnAdd(invalidObj, false)

	select {
	case name := <-dgCtrl.ch:
		t.Fatalf("Did not expect to receive name for invalid object, but got: %s", name)
	case <-time.After(100 * time.Millisecond):
		// Expected - no name should be sent for invalid objects
	}
}
