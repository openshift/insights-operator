package utils

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func NestedStringWrapper(obj map[string]interface{}, fields ...string) (string, error) {
	s, ok, err := unstructured.NestedString(obj, fields...)
	if !ok {
		return "", fmt.Errorf("can't find %s", fields)
	}
	if err != nil {
		return "", err
	}

	return s, nil
}

func NestedSliceWrapper(obj map[string]interface{}, fields ...string) ([]interface{}, error) {
	s, ok, err := unstructured.NestedSlice(obj, fields...)
	if !ok {
		return nil, fmt.Errorf("can't find %s", fields)
	}
	if err != nil {
		return nil, err
	}

	return s, nil
}
