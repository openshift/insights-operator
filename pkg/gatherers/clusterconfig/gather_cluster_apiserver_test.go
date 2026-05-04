package clusterconfig

import (
	"context"
	"testing"

	v1 "github.com/openshift/api/config/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_GatherClusterAPIServer(t *testing.T) {
	// Unit Tests
	testCases := []struct {
		name      string
		apiserver *v1.APIServer
		itemCount int
		errCount  int
	}{
		{
			name:      "No APIServer found returns no error",
			apiserver: &v1.APIServer{ObjectMeta: metav1.ObjectMeta{Name: "mock"}},
			itemCount: 0,
			errCount:  0,
		},
		{
			name:      "cluster APIServer is properly recorded",
			apiserver: &v1.APIServer{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}},
			itemCount: 1,
			errCount:  0,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Given
			configClient := configfake.NewClientset(tc.apiserver)

			// When
			records, err := clusterAPIServer{}.gather(context.Background(), configClient.ConfigV1().APIServers())

			// Assert
			assert.Len(t, err, tc.errCount)
			assert.Len(t, records, tc.itemCount)
			if tc.itemCount > 0 {
				assert.Equal(t, "config/apiserver", records[0].Name)
			}
		})
	}
}
