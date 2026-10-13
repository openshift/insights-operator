package clusterconfig

import (
	"context"
	"testing"

	v1 "github.com/openshift/api/config/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
)

func Test_gatherClusterNetwork(t *testing.T) {
	// Unit Tests
	testCases := []struct {
		name       string
		network    *v1.Network
		result     []record.Record
		errorCount int
	}{
		{
			name:    "Retrieving network returns record of that network and no errors",
			network: &v1.Network{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}},
			result: []record.Record{
				{
					Name: "config/network",
					Item: record.ResourceMarshaller{
						Resource: &v1.Network{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}},
					},
				},
			},
			errorCount: 0,
		},
		{
			name:       "Retrieving no network returns no error/no record",
			network:    &v1.Network{},
			result:     nil,
			errorCount: 0,
		},
	}

	for _, testCase := range testCases {
		tc := testCase
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Given
			configClient := configfake.NewSimpleClientset(tc.network)

			// When
			test, errs := gatherClusterNetwork(context.Background(), configClient.ConfigV1())

			// Assert
			assert.Equal(t, tc.result, test)
			assert.Len(t, errs, tc.errorCount)
		})
	}
}
