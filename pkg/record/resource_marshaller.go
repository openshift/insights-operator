package record

import (
	"context"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// managedFieldsSetter is a universal interface of
// resources that implement the SetManagedFields method.
type managedFieldsSetter interface {
	SetManagedFields(managedFields []metav1.ManagedFieldsEntry)
}

// ResourceMarshaller marshals a Kubernetes/OpenShift resource into a JSON format.
// It performs a cleanup of the resource before the marshalling to reduce resource disk/memory size.
type ResourceMarshaller struct {
	Resource managedFieldsSetter
}

// Marshal cleans up the resource structure by removing unnecessary fields
// and converts it into a JSON format using the default serializer.
func (m ResourceMarshaller) Marshal(_ context.Context) ([]byte, error) {
	// If the resource passed to the marshaller is structured (e.g., Pod,
	// Node, NetNamespace), or if the resource is passed as the raw
	// unstructured.Unstructured struct instance (which has the same methods
	// available as regular structured resources), it is possible to remove
	// the managedFields by making a single call to the appropriate method.
	m.Resource.SetManagedFields(nil)
	return json.Marshal(m.Resource)
}

// GetExtension returns the file extension that should be used for marshalled resources (json).
func (m ResourceMarshaller) GetExtension() string {
	return "json"
}
