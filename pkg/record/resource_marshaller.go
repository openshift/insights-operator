package record

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// managedFieldsSetter is a universal interface of
// resources that implement the SetManagedFields method.
type managedFieldsSetter interface {
	SetManagedFields(managedFields []metav1.ManagedFieldsEntry)
}

// ResourceMarshaller serializes a Kubernetes/OpenShift resource into a JSON format.
// It performs cleanup of the resource before serialization to reduce resource disk/memory size.
type ResourceMarshaller struct {
	Resource managedFieldsSetter
}

// Marshal cleans up the resource structure by removing unnecessary fields
// and converts it into a JSON format using the default serializer.
func (m ResourceMarshaller) Marshal() ([]byte, error) {
	// If the resource passed to the marshaller is structured (e.g., Pod,
	// Node, NetNamespace), or if the resource is passed as the raw
	// unstructured.Unstructured struct instance (which has the same methods
	// available as regular structured resources), it is possible to remove
	// the managedFields by making a single call to the appropriate method.
	m.Resource.SetManagedFields(nil)
	return json.Marshal(m.Resource)
}

// GetExtension returns the file extension that should be used for serialized resources (JSON).
func (m ResourceMarshaller) GetExtension() string {
	return JSONExtension
}
