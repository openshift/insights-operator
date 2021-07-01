package record

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var managedFieldsCheckBytes = []byte(`"managedFields":`)

func Test_ResourceMarshaller_GetExtension(t *testing.T) {
	if ext := (ResourceMarshaller{}).GetExtension(); ext != "json" {
		t.Fatalf(`unexpected extension returned by ResourceMarshaller: %q (expected "json")`, ext)
	}
}

func Test_ResourceMarshaller_MarshalUnstructured(t *testing.T) {
	unstr := unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"managedFields": map[string]interface{}{
				"key":       "value",
				"secondKey": "secondValue",
			},
		},
	}}

	jsonBytesDirect, err := json.Marshal(&unstr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(jsonBytesDirect, managedFieldsCheckBytes) {
		t.Fatal("managedFields field is missing from the resource (even before its removal)")
	}

	rm := ResourceMarshaller{Resource: &unstr}
	jsonBytesRM, err := rm.Marshal(context.TODO())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(jsonBytesRM, managedFieldsCheckBytes) {
		t.Fatal("managedFields field has not been removed from the resource")
	}

	if len(jsonBytesRM) >= len(jsonBytesDirect) {
		t.Fatalf("JSON from ResourceMarshaller is not smaller than directly marshalled JSON (%d >= %d)", len(jsonBytesRM), len(jsonBytesDirect))
	}
}

func Test_ResourceMarshaller_MarshalPod(t *testing.T) {
	pod := corev1.Pod{ObjectMeta: v1.ObjectMeta{ManagedFields: []v1.ManagedFieldsEntry{
		{
			Manager:    "manager",
			FieldsType: "string",
			Operation:  v1.ManagedFieldsOperationUpdate,
			APIVersion: "1",
			Time:       &v1.Time{},
		},
	}}}

	jsonBytesDirect, err := json.Marshal(&pod)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(jsonBytesDirect, managedFieldsCheckBytes) {
		t.Fatal("managedFields field is missing from the resource (even before its removal)")
	}

	rm := ResourceMarshaller{Resource: &pod}
	jsonBytesRM, err := rm.Marshal(context.TODO())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(jsonBytesRM, managedFieldsCheckBytes) {
		t.Fatal("managedFields field has not been removed from the resource")
	}

	if len(jsonBytesRM) >= len(jsonBytesDirect) {
		t.Fatalf("JSON from ResourceMarshaller is not smaller than directly marshalled JSON (%d >= %d)", len(jsonBytesRM), len(jsonBytesDirect))
	}
}
