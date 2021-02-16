package clusterconfig

import (
	"context"
	"testing"

	imageregistryv1 "github.com/openshift/api/imageregistry/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	registryv1 "github.com/openshift/api/imageregistry/v1"
	imageregistryfake "github.com/openshift/client-go/imageregistry/clientset/versioned/fake"
)

func TestGatherClusterPruner(t *testing.T) {
	tests := []struct {
		name            string
		inputObj        runtime.Object
		expectedRecords int
		evalOutput      func(t *testing.T, obj *imageregistryv1.ImagePruner)
	}{
		{
			name: "not found",
			inputObj: &imageregistryv1.ImagePruner{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pruner-i-dont-care-about",
				},
			},
		},
		{
			name:            "simple image pruner",
			expectedRecords: 1,
			inputObj: &imageregistryv1.ImagePruner{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Spec: imageregistryv1.ImagePrunerSpec{
					Schedule: "0 0 * * *",
				},
			},
			evalOutput: func(t *testing.T, obj *imageregistryv1.ImagePruner) {
				if obj.Name != "cluster" {
					t.Errorf("received wrong prunner: %+v", obj)
					return
				}
				if obj.Spec.Schedule != "0 0 * * *" {
					t.Errorf("unexpected spec.schedule: %q", obj.Spec.Schedule)
				}
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := imageregistryfake.NewSimpleClientset(test.inputObj)
			ctx := context.Background()
			records, errs := gatherClusterImagePruner(ctx, client.ImageregistryV1())
			if len(errs) > 0 {
				t.Errorf("unexpected errors: %#v", errs)
				return
			}
			if numRecords := len(records); numRecords != test.expectedRecords {
				t.Errorf("expected one record, got %d", numRecords)
				return
			}
			if test.expectedRecords == 0 {
				return
			}
			if expectedRecordName := "config/clusteroperator/imageregistry.operator.openshift.io/imagepruner/cluster"; records[0].Name != expectedRecordName {
				t.Errorf("expected %q record name, got %q", expectedRecordName, records[0].Name)
				return
			}
			item := records[0].Item
			itemBytes, err := item.Marshal(context.TODO())
			if err != nil {
				t.Fatalf("unable to marshal config: %v", err)
			}
			registryScheme := runtime.NewScheme()
			utilruntime.Must(registryv1.AddToScheme(registryScheme))
			registrySerializer := serializer.NewCodecFactory(registryScheme)
			var output imageregistryv1.ImagePruner
			obj, _, err := registrySerializer.LegacyCodec(imageregistryv1.SchemeGroupVersion).Decode(itemBytes, nil, &output)
			if err != nil {
				t.Fatalf("failed to decode object: %v", err)
			}
			test.evalOutput(t, obj.(*imageregistryv1.ImagePruner))
		})
	}
}
