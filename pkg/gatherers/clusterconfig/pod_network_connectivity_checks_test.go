package clusterconfig

import (
	"context"
	"testing"
	"time"

	controlplanev1alpha1 "github.com/openshift/api/operatorcontrolplane/v1alpha1"
	ocpCliFake2 "github.com/openshift/client-go/operatorcontrolplane/clientset/versioned/fake"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const podnetworkconnectivitychecksPath = "config/podnetworkconnectivitychecks"

func Test_PNCC(t *testing.T) {
	testPncc := controlplanev1alpha1.PodNetworkConnectivityCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name: "example-pncc",
		},
		Status: controlplanev1alpha1.PodNetworkConnectivityCheckStatus{
			Failures: []controlplanev1alpha1.LogEntry{
				{
					Success: false,
					Reason:  "TestReason",
					Message: "TestMessage",
					Start: metav1.Time{
						Time: time.Now().Add(-5 * time.Minute),
					},
				},
			},
		},
	}
	fakeOCPInterface := ocpCliFake2.NewSimpleClientset()
	// Check before creating the PNCC.
	records, errs := gatherPNCC(context.Background(), fakeOCPInterface.ControlplaneV1alpha1())
	assert.Len(t, errs, 0, "unexpected errors in the first run: %#v", errs)
	assert.Equal(t, 1, len(records), "unexpected number or records in the first run: %d", len(records))
	assert.Equal(t, podnetworkconnectivitychecksPath, records[0].Name)

	recItem, ok := records[0].Item.(record.JSONMarshaller)
	assert.True(t, ok, "unexpected type of record item in the first run: %q", records[0].Name)
	assert.Equal(t, map[string]map[string]time.Time{}, recItem.Object)

	// Create the PNCC resource.
	_, err := fakeOCPInterface.ControlplaneV1alpha1().
		PodNetworkConnectivityChecks("example-namespace").Create(context.Background(), &testPncc, metav1.CreateOptions{})
	assert.NoError(t, err)
	// Check after creating the PNCC.
	records, errs = gatherPNCC(context.Background(), fakeOCPInterface.ControlplaneV1alpha1())
	assert.Len(t, errs, 0, "unexpected errors in the second run: %#v", errs)
	assert.Equal(t, 1, len(records), "unexpected number or records in the second run: %d", len(records))
	assert.Equal(t, podnetworkconnectivitychecksPath, records[0].Name)

	recItem, ok = records[0].Item.(record.JSONMarshaller)
	assert.True(t, ok, "unexpected type of record item in second first run: %q", records[0].Name)
	assert.Equal(t, map[string]map[string]time.Time{"TestReason": {"TestMessage": testPncc.Status.Failures[0].Start.Time}}, recItem.Object)
}
