package clusterconfig

import (
	"context"
	"testing"

	v1 "github.com/openshift/api/config/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_GatherClusterInfrastructure(t *testing.T) {
	testCases := []struct {
		name       string
		infra      *v1.Infrastructure
		result     []record.Record
		errorCount int
	}{
		{
			name: "Retrieving infrastructure returns record of that infrastructure and no errors",
			infra: &v1.Infrastructure{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Status:     v1.InfrastructureStatus{PlatformStatus: &v1.PlatformStatus{}},
			},
			result: []record.Record{
				{
					Name: "config/infrastructure",
					Item: record.ResourceMarshaller{
						Resource: &v1.Infrastructure{
							ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
							Status:     v1.InfrastructureStatus{PlatformStatus: &v1.PlatformStatus{}},
						},
					},
				},
			},
		},
		{
			name:   "Retrieving no infrastructure returns no error/no record",
			infra:  &v1.Infrastructure{},
			result: nil,
		},
		{
			name: "Check 'Status' InfrastructureName property value returns obfuscated",
			infra: &v1.Infrastructure{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Status: v1.InfrastructureStatus{
					InfrastructureName: "test",
					PlatformStatus:     &v1.PlatformStatus{},
				},
			},
			result: []record.Record{
				{
					Name: "config/infrastructure",
					Item: record.ResourceMarshaller{
						Resource: &v1.Infrastructure{
							ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
							Status: v1.InfrastructureStatus{
								InfrastructureName: "xxxx",
								PlatformStatus:     &v1.PlatformStatus{}},
						},
					},
				},
			},
			errorCount: 0,
		},
	}

	for _, testCase := range testCases {
		tc := testCase
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Given
			configClient := configfake.NewSimpleClientset(tc.infra)

			// When
			test, errs := gatherClusterInfrastructure(context.Background(), configClient.ConfigV1())

			// Assert
			assert.Equal(t, tc.result, test)
			assert.Len(t, errs, tc.errorCount)
		})
	}
}
