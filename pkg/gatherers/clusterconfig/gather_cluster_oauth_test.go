package clusterconfig

import (
	"context"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_gatherClusterOAuth(t *testing.T) {
	tests := []struct {
		name            string
		oAuthDefinition *configv1.OAuth
		wantRecords     []record.Record
		wantErrCount    int
	}{
		{
			name:            "succesful retrieval oauth",
			oAuthDefinition: &configv1.OAuth{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}},
			wantRecords: []record.Record{
				{
					Name: "config/oauth",
					Item: record.ResourceMarshaller{
						Resource: &configv1.OAuth{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}},
					},
				},
			},
			wantErrCount: 0,
		},
		{
			name:            "no oauth",
			oAuthDefinition: &configv1.OAuth{},
			wantRecords:     []record.Record(nil),
			wantErrCount:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configClient := configfake.NewSimpleClientset(tt.oAuthDefinition)
			records, errs := gatherClusterOAuth(context.TODO(), configClient.ConfigV1())
			assert.Equal(t, tt.wantRecords, records)
			assert.Len(t, errs, tt.wantErrCount)
		})
	}
}
