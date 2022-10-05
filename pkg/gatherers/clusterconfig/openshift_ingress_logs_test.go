package clusterconfig

import (
	"context"
	"errors"
	"testing"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

func Test_GatherOpenShiftIngressLogs(t *testing.T) {
	tests := []struct {
		name    string
		fnMock  func() ([]record.Record, error)
		wantErr bool
		want    []record.Record
	}{
		{
			name: "case collectLogsFromContainer returns an error",
			fnMock: func() ([]record.Record, error) {
				return []record.Record{}, errors.New("collectLogsFromContainer error")
			},
			wantErr: true,
		},
		{
			name: "case collectLogsFromContainer returns a record collection",
			fnMock: func() ([]record.Record, error) {
				return []record.Record{{Name: "mock record"}}, nil
			},
			want: []record.Record{{Name: "mock record"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Mocking dependencies
			mockGatherer := Gatherer{gatherProtoKubeConfig: &rest.Config{}}
			ingressCollectLogsFromContainers = func(
				ctx context.Context,
				coreClient v1.CoreV1Interface,
				containersFilter common.LogContainersFilter,
				messagesFilter common.LogMessagesFilter,
				buildLogFileName func(namespace string, podName string, containerName string) string,
			) ([]record.Record, error) {
				return test.fnMock()
			}

			// Given
			records, err := mockGatherer.GatherOpenShiftIngressLogs(context.TODO())

			// Assertions
			if test.wantErr {
				assert.Len(t, err, 1)
				assert.Error(t, err[0])
			}
			assert.ElementsMatch(t, test.want, records)
		})
	}
}
