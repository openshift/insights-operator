package clusterconfig

import (
	"context"
	"testing"

	imageregistryv1 "github.com/openshift/api/imageregistry/v1"
	"github.com/openshift/insights-operator/pkg/record"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	imageregistryfake "github.com/openshift/client-go/imageregistry/clientset/versioned/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

//nolint: goconst, funlen, gocyclo
func Test_ImageRegistry_Gather(t *testing.T) {
	tests := []struct {
		name       string
		inputObj   *imageregistryv1.Config
		evalOutput func(t *testing.T, obj *imageregistryv1.Config)
	}{
		{
			name: "httpSecret",
			inputObj: &imageregistryv1.Config{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Spec: imageregistryv1.ImageRegistrySpec{
					HTTPSecret: "secret",
				},
			},
			evalOutput: func(t *testing.T, obj *imageregistryv1.Config) {
				if obj.Spec.HTTPSecret != "xxxxxx" {
					t.Errorf("expected HTTPSecret anonymized, got %q", obj.Spec.HTTPSecret)
				}
			},
		},
		{
			name: "s3",
			inputObj: &imageregistryv1.Config{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Spec: imageregistryv1.ImageRegistrySpec{
					Storage: imageregistryv1.ImageRegistryConfigStorage{
						S3: &imageregistryv1.ImageRegistryConfigStorageS3{
							Bucket:         "foo",
							Region:         "bar",
							RegionEndpoint: "point",
							KeyID:          "key",
						},
					},
				},
			},
			evalOutput: func(t *testing.T, obj *imageregistryv1.Config) {
				if obj.Spec.Storage.S3.Bucket != "xxx" {
					t.Errorf("expected s3 bucket anonymized, got %q", obj.Spec.Storage.S3.Bucket)
				}
				if obj.Spec.Storage.S3.Region != "xxx" {
					t.Errorf("expected s3 region anonymized, got %q", obj.Spec.Storage.S3.Region)
				}
				if obj.Spec.Storage.S3.RegionEndpoint != "xxxxx" {
					t.Errorf("expected s3 region endpoint anonymized, got %q", obj.Spec.Storage.S3.RegionEndpoint)
				}
				if obj.Spec.Storage.S3.KeyID != "xxx" {
					t.Errorf("expected s3 keyID anonymized, got %q", obj.Spec.Storage.S3.KeyID)
				}
			},
		},
		{
			name: "azure",
			inputObj: &imageregistryv1.Config{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Spec: imageregistryv1.ImageRegistrySpec{
					Storage: imageregistryv1.ImageRegistryConfigStorage{
						Azure: &imageregistryv1.ImageRegistryConfigStorageAzure{
							AccountName: "account",
							Container:   "container",
						},
					},
				},
			},
			evalOutput: func(t *testing.T, obj *imageregistryv1.Config) {
				if obj.Spec.Storage.Azure.AccountName != "xxxxxxx" {
					t.Errorf("expected azure account name anonymized, got %q", obj.Spec.Storage.Azure.AccountName)
				}
				if obj.Spec.Storage.Azure.Container == "xxxxxxx" {
					t.Errorf("expected azure container anonymized, got %q", obj.Spec.Storage.Azure.Container)
				}
			},
		},
		{
			name: "gcs",
			inputObj: &imageregistryv1.Config{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Spec: imageregistryv1.ImageRegistrySpec{
					Storage: imageregistryv1.ImageRegistryConfigStorage{
						GCS: &imageregistryv1.ImageRegistryConfigStorageGCS{
							Bucket:    "bucket",
							Region:    "region",
							ProjectID: "foo",
							KeyID:     "bar",
						},
					},
				},
			},
			evalOutput: func(t *testing.T, obj *imageregistryv1.Config) {
				if obj.Spec.Storage.GCS.Bucket != "xxxxxx" {
					t.Errorf("expected gcs bucket anonymized, got %q", obj.Spec.Storage.GCS.Bucket)
				}
				if obj.Spec.Storage.GCS.ProjectID != "xxx" {
					t.Errorf("expected gcs projectID endpoint anonymized, got %q", obj.Spec.Storage.GCS.ProjectID)
				}
				if obj.Spec.Storage.GCS.KeyID != "xxx" {
					t.Errorf("expected gcs keyID anonymized, got %q", obj.Spec.Storage.GCS.KeyID)
				}
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := imageregistryfake.NewSimpleClientset(test.inputObj)
			coreClient := kubefake.NewSimpleClientset()
			ctx := context.Background()
			records, errs := gatherClusterImageRegistry(ctx, client.ImageregistryV1(), coreClient.CoreV1())
			if len(errs) > 0 {
				t.Errorf("unexpected errors: %#v", errs)
				return
			}
			if numRecords := len(records); numRecords != 1 {
				t.Errorf("expected one record, got %d", numRecords)
				return
			}
			expectedRecordName := "config/clusteroperator/imageregistry.operator.openshift.io/config/cluster"
			if records[0].Name != expectedRecordName {
				t.Errorf("expected %q record name, got %q", expectedRecordName, records[0].Name)
				return
			}
			item := records[0].Item
			_, err := item.Marshal(context.TODO())
			if err != nil {
				t.Fatalf("unable to marshal config: %v", err)
			}
			obj, ok := item.(record.ResourceMarshaller).Resource.(*imageregistryv1.Config)
			if !ok {
				t.Fatalf("failed to decode object")
			}
			test.evalOutput(t, obj)
		})
	}
}
