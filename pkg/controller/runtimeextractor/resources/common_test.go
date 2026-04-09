package resources

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func Test_loadYAMLResource(t *testing.T) {
	tests := []struct {
		name         string
		yamlData     []byte
		resourceType resourceType
		wantErr      bool
	}{
		{
			name: "valid YAML for ConfigMap",
			yamlData: []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
  namespace: test-ns
data:
  key: value
`),
			resourceType: resourceTypeConfigMap,
			wantErr:      false,
		},
		{
			name:         "invalid YAML",
			yamlData:     []byte(`invalid: yaml: data: [[[`),
			resourceType: resourceTypeConfigMap,
			wantErr:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var obj interface{}
			err := loadYAMLResource(tt.yamlData, &obj, tt.resourceType)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to unmarshal runtime extractor")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_deleteResource(t *testing.T) {
	tests := []struct {
		name         string
		deleteFn     func() error
		namespace    string
		resourceName string
		resourceType resourceType
		wantErr      bool
	}{
		{
			name: "successful delete",
			deleteFn: func() error {
				return nil
			},
			namespace:    "test-ns",
			resourceName: "test-resource",
			resourceType: resourceTypeConfigMap,
			wantErr:      false,
		},
		{
			name: "resource not found - no error",
			deleteFn: func() error {
				return apierrors.NewNotFound(schema.GroupResource{Resource: "configmaps"}, "test-resource")
			},
			namespace:    "test-ns",
			resourceName: "test-resource",
			resourceType: resourceTypeConfigMap,
			wantErr:      false,
		},
		{
			name: "delete error",
			deleteFn: func() error {
				return apierrors.NewInternalError(errors.New("delete failed"))
			},
			namespace:    "test-ns",
			resourceName: "test-resource",
			resourceType: resourceTypeConfigMap,
			wantErr:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := deleteResource(tt.deleteFn, tt.namespace, tt.resourceName, tt.resourceType)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to delete runtime extractor")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_resourceExists(t *testing.T) {
	tests := []struct {
		name         string
		getFn        func() (interface{}, error)
		namespace    string
		resourceName string
		resourceType resourceType
		want         bool
	}{
		{
			name: "resource exists",
			getFn: func() (interface{}, error) {
				return &struct{}{}, nil
			},
			namespace:    "test-ns",
			resourceName: "test-resource",
			resourceType: resourceTypeConfigMap,
			want:         true,
		},
		{
			name: "resource not found",
			getFn: func() (interface{}, error) {
				return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "configmaps"}, "test-resource")
			},
			namespace:    "test-ns",
			resourceName: "test-resource",
			resourceType: resourceTypeConfigMap,
			want:         false,
		},
		{
			name: "get error returns false",
			getFn: func() (interface{}, error) {
				return nil, apierrors.NewServiceUnavailable("service unavailable")
			},
			namespace:    "test-ns",
			resourceName: "test-resource",
			resourceType: resourceTypeService,
			want:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resourceExists(tt.getFn, tt.namespace, tt.resourceName, tt.resourceType)

			assert.Equal(t, tt.want, got)
		})
	}
}
