package clusterconfig

import (
	"regexp"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/anonymization"
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

// This function goes over all fields in the provided map and anonymize all IPv4 addresses which will find
func anonymizeIpAddresses(data map[string]interface{}) map[string]interface{} {
	ipv4Regex := regexp.MustCompile(anonymization.Ipv4Regex)
	for fieldName, fieldValue := range data {
		switch currentValue := fieldValue.(type) {
		case map[string]interface{}:
			data[fieldName] = anonymizeIpAddresses(currentValue)
		default:
			currentValueStr, _ := currentValue.(string)
			isIpv4 := ipv4Regex.FindStringIndex(currentValueStr)
			if isIpv4 != nil {
				data[fieldName] = anonymize.String(currentValueStr)
			}
		}
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
			unstructured.SetNestedField(data, "xxx", fieldToAnonymize...)
		} else {
			unstructured.SetNestedField(data, anonymize.String(fieldValueStr), fieldToAnonymize...)
		}
	}
	return data
}
