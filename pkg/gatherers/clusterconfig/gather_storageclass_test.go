package clusterconfig

import (
	"context"
	"testing"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGatherStorageClasses(t *testing.T) {
	tests := []struct {
		name           string
		storageClasses []storagev1.StorageClass
		wantRecords    []record.Record
		wantErrCount   int
	}{
		{
			name: "Successful retrieval of storageclasses",
			storageClasses: []storagev1.StorageClass{
				{
					ObjectMeta:  metav1.ObjectMeta{Name: "standard-csi"},
					Provisioner: "pd.csi.storage.gke.io",
					Parameters: map[string]string{
						"replication-type": "none",
						"type":             "pd-standard",
					},
				},
			},
			wantRecords: []record.Record{
				{
					Name: "config/storage/storageclasses/standard-csi",
					Item: record.ResourceMarshaller{
						Resource: &storagev1.StorageClass{
							ObjectMeta:  metav1.ObjectMeta{Name: "standard-csi"},
							Provisioner: "pd.csi.storage.gke.io",
							Parameters: map[string]string{
								"replication-type": "none",
								"type":             "pd-standard",
							},
						},
					},
				},
			},
			wantErrCount: 0,
		},
		{
			name:           "Retrieval no storageclasses items",
			storageClasses: nil,
			wantRecords:    nil,
			wantErrCount:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fake client and store it in a variable
			kubeClient := fake.NewSimpleClientset(&storagev1.StorageClassList{
				Items: tt.storageClasses,
			})

			// Call the gatherStorageClasses function with the fake client
			records, errs := gatherStorageClasses(context.Background(), kubeClient.StorageV1())

			// Verify the results
			assert.Equal(t, tt.wantRecords, records)
			assert.Len(t, errs, tt.wantErrCount)
		})
	}
}
