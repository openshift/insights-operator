package clusterconfig

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openshift/insights-operator/pkg/utils/marshal"

	"github.com/stretchr/testify/assert"
)

func Test_gatherSilencedAlerts(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		dummyData := []byte(`{"silenced": true, "active": false, "inhibited": false}`)
		w.Write(dummyData)
	}))
	defer ts.Close()
	rc := testRESTClient(t, ts)

	records, errs := gatherSilencedAlerts(context.Background(), rc)
	assert.Len(t, errs, 0)
	assert.NotEmpty(t, records)

	assert.Equal(t, "config/silenced_alerts.json", records[0].Name)
	assert.NotNil(t, records[0].Captured)
	assert.Equal(t, marshal.RawByte([]byte(`{"silenced": true, "active": false, "inhibited": false}`)), records[0].Item)
}
