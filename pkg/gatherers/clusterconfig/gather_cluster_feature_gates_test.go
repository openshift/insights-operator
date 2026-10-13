package clusterconfig

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/openshift/api/config/v1"

	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
)

func Test_gatherClusterFeatureGates(t *testing.T) {
	tests := []struct {
		name     string
		feature  *v1.FeatureGate
		result   []record.Record
		errCount int
	}{
		{
			name: "Retrieving featuregate returns record of that featuregate and no errors",
			feature: &v1.FeatureGate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
			},
			result: []record.Record{
				{
					Name: "config/featuregate",
					Item: record.ResourceMarshaller{
						Resource: &v1.FeatureGate{
							ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
						},
					},
				},
			},
			errCount: 0,
		},
		{
			name:    "Retrieving no featuregate returns no error/no record",
			feature: &v1.FeatureGate{},
			result:  nil,
		},
	}
	for _, testCase := range tests {
		tc := testCase
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Given
			configClient := configfake.NewSimpleClientset(tc.feature)

			// When
			got, gotErrs := gatherClusterFeatureGates(context.Background(), configClient.ConfigV1())

			// Assert
			assert.Equal(t, tc.result, got)
			assert.Len(t, gotErrs, tc.errCount)
		})
	}
}
