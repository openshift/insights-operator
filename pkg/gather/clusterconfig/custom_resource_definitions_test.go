package clusterconfig

import (
	"context"
	"reflect"
	"testing"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apixv1beta1clientfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCollectVolumeSnapshotCRD(t *testing.T) {
	expectedRecords := map[string]v1beta1.CustomResourceDefinition{
		"config/crd/volumesnapshots.snapshot.storage.k8s.io":        {ObjectMeta: metav1.ObjectMeta{Name: "volumesnapshots.snapshot.storage.k8s.io"}},
		"config/crd/volumesnapshotcontents.snapshot.storage.k8s.io": {ObjectMeta: metav1.ObjectMeta{Name: "volumesnapshotcontents.snapshot.storage.k8s.io"}},
	}

	crdNames := []string{
		"unrelated.custom.resource.definition.k8s.io",
		"volumesnapshots.snapshot.storage.k8s.io",
		"volumesnapshotcontents.snapshot.storage.k8s.io",
		"another.irrelevant.custom.resource.definition.k8s.io",
		"this.should.not.be.gathered.k8s.io",
	}

	crdClientset := apixv1beta1clientfake.NewSimpleClientset()

	for _, name := range crdNames {
		crdClientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(context.Background(), &v1beta1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: name},
		}, metav1.CreateOptions{})
	}

	gatherer := &Gatherer{crdClient: crdClientset.ApiextensionsV1beta1()}
	records, errs := GatherCRD(gatherer)()
	if len(errs) != 0 {
		t.Fatalf("gather CRDs resulted in error: %#v", errs)
	}

	if len(records) != len(expectedRecords) {
		t.Fatalf("unexpected number of records gathered: %d (expected %d)", len(records), len(expectedRecords))
	}

	for _, rec := range records {
		if expectedItem, ok := expectedRecords[rec.Name]; !ok {
			t.Fatalf("unexpected gathered record name: %q", rec.Name)
		} else if reflect.DeepEqual(rec.Item, expectedItem) {
			t.Fatalf("gathered record %q has different item value than unexpected", rec.Name)
		}
	}
}
