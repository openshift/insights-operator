package clusterconfig

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/rest"

	"github.com/stretchr/testify/assert"
)

type mockTSDBClient struct {
	data []byte
}

func (c *mockTSDBClient) RestClient(t *testing.T) *rest.RESTClient {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if c.data == nil {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(c.data)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
			}
		}
	}))

	baseURL, err := url.Parse(ts.URL)
	assert.NoError(t, err, "failed to parse server URL")
	client, err := rest.NewRESTClient(baseURL, "", rest.ClientContentConfig{}, nil, nil)
	assert.NoError(t, err, "failed to create the client")

	return client
}

func TestGatherPrometheusTSDBStatus(t *testing.T) {
	testCases := []struct {
		name          string
		metricsClient *mockTSDBClient
		wantRecords   []record.Record
		wantErrors    []error
	}{
		{
			name:          "get prometheus tsdb status",
			metricsClient: &mockTSDBClient{data: []byte(`test`)},
			wantRecords: []record.Record{
				{
					Name: "config/tsdb.json",
					Item: marshal.RawByte(`test`),
				},
			},
			wantErrors: nil,
		},
		{
			name:          "fail to get prometheus tsdb status",
			metricsClient: &mockTSDBClient{data: nil},
			wantRecords:   nil,
			wantErrors: []error{
				&errors.StatusError{ErrStatus: metav1.Status{
					Status:  metav1.StatusFailure,
					Message: "the server could not find the requested resource",
					Reason:  metav1.StatusReasonNotFound,
					Details: &metav1.StatusDetails{
						Causes: []metav1.StatusCause{
							{
								Type:    metav1.CauseTypeUnexpectedServerResponse,
								Message: "",
								Field:   "",
							},
						},
					},
					Code: http.StatusNotFound,
				}},
			},
		},
	}
	for _, testCase := range testCases {
		tc := testCase
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			records, errs := gatherPrometheusTSDBStatus(context.Background(), tc.metricsClient.RestClient(t))
			assert.Equal(t, tc.wantRecords, records)
			assert.Equal(t, tc.wantErrors, errs)
		})
	}
}
