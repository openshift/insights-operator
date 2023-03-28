package clusterconfig

import (
	"context"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
)

func Test_gatherClusterIngress(t *testing.T) {
	tests := []struct {
		name              string
		ingressDefinition *configv1.Ingress
		wantRecords       []record.Record
		wantErrCount      int
	}{
		{
			name:              "successful retrieval cluster ingress",
			ingressDefinition: &configv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}},
			wantRecords: []record.Record{
				{
					Name: "config/ingress",
					Item: record.ResourceMarshaller{
						Resource: &configv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}},
					},
				},
			},
			wantErrCount: 0,
		},
		{
			name:              "failed retrieval cluster ingress",
			ingressDefinition: &configv1.Ingress{},
			wantRecords:       nil,
			wantErrCount:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configClient := configfake.NewSimpleClientset(tt.ingressDefinition)
			records, errs := gatherClusterIngress(context.TODO(), configClient.ConfigV1())
			assert.Equal(t, tt.wantRecords, records)
			assert.Len(t, errs, tt.wantErrCount)
		})
	}
}
