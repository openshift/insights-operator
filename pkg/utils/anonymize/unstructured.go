package anonymize

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// UnstructuredNestedStringField anonymizes the nested field in the unstructured object
// or returns error if it is not possible
func UnstructuredNestedStringField(data map[string]interface{}, fields ...string) error {
	value, found, err := unstructured.NestedFieldCopy(data, fields...)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("unable to find field '%v'", fields)
	}

	valueStr, _ := value.(string)
	return unstructured.SetNestedField(data, String(valueStr), fields...)
}
