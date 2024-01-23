package clusterconfig

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"

	v1 "github.com/openshift/api/config/v1"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
)

func Test_gatherClusterAuthentication(t *testing.T) {
	tests := []struct {
		name           string
		authentication *v1.Authentication
		result         []record.Record
		errCount       int
	}{
		{
			name: "Retrieving authentication returns record of that authentication and no errors",
			authentication: &v1.Authentication{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
			},
			result: []record.Record{
				{
					Name: "config/authentication",
					Item: record.ResourceMarshaller{
						Resource: &v1.Authentication{
							ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
						},
					},
				},
			},
			errCount: 0,
		},
		{
			name:           "Retrieving no authentication returns no error/no records",
			authentication: &v1.Authentication{},
			result:         nil,
		},
	}
	for _, testCase := range tests {
		tc := testCase
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Given
			configClient := configfake.NewSimpleClientset(tc.authentication)

			// When
			got, gotErrs := gatherClusterAuthentication(context.Background(), configClient.ConfigV1())

			// Assert
			assert.Equal(t, tc.result, got)
			assert.Len(t, gotErrs, tc.errCount)
		})
	}
}
