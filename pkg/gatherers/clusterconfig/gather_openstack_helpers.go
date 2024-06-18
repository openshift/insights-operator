package clusterconfig

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/utils/anonymize"
)

// This function removes from the CR which is represented by the map[string]interface{} data,
// fields listed in the 'fieldsToRemove' list. Each field has to have full path provided to be removed.
func removeFields(data map[string]interface{}, fieldsToRemove [][]string) map[string]interface{} {
	for _, fieldToRemove := range fieldsToRemove {
		unstructured.RemoveNestedField(data, fieldToRemove...)
	}
	return data
}

func anonymizeFields(data map[string]interface{}, fieldsToAnonymize [][]string) map[string]interface{} {
	for _, fieldToAnonymize := range fieldsToAnonymize {
		fieldValue, found, err := unstructured.NestedFieldCopy(data, fieldToAnonymize...)
		if err != nil {
			klog.Infof("error during anonymization of field '%v': error: %s", fieldToAnonymize, err)
			continue
		}
		if !found {
			klog.Infof("field '%v' not found", fieldToAnonymize)
			continue
		}
		fieldValueStr, _ := fieldValue.(string)
		if len(fieldValueStr) == 0 {
			// in case if field contains e.g. map[string]interface{} or list
			// so that its string representation is empty, it is easier to just
			// put 'xxx' in that place
			err := unstructured.SetNestedField(data, "xxx", fieldToAnonymize...)
			if err != nil {
				klog.Infof("error during setting annonymized data in the nested field '%v': error: '%s'", fieldToAnonymize, err)
				continue
			}
		} else {
			err := unstructured.SetNestedField(data, anonymize.String(fieldValueStr), fieldToAnonymize...)
			if err != nil {
				klog.Infof("error during setting annonymized data in the nested field '%v': error: '%s'", fieldToAnonymize, err)
				continue
			}
		}
	}
	return data
}
