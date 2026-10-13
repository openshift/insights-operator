package clusterconfig

import (
	"context"
	"testing"

	v1 "github.com/openshift/api/config/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1/fake"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
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
			clientset := fake.NewSimpleClientset()
			configv1 := configfake.FakeConfigV1{Fake: &clientset.Fake}
			configv1.APIServers().Create(context.Background(), tc.apiserver, metav1.CreateOptions{}) // nolint: errcheck

			// When
			records, err := clusterAPIServer{}.gather(context.Background(), configv1.APIServers())

			// Assert
			assert.Len(t, err, tc.errCount)
			assert.Len(t, records, tc.itemCount)
			if tc.itemCount > 0 {
				assert.Equal(t, "config/apiserver", records[0].Name)
			}
		})
	}
}
