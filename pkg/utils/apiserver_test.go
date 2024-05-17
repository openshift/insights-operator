package utils

import (
	"context"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_GetClusterAPIServerInfo(t *testing.T) {
	type testCase struct {
		name     string
		cluster  configv1.Infrastructure
		expected []string
	}

	testCases := []testCase{
		{
			name:     "Base infrastructure does not cause an error",
			cluster:  configv1.Infrastructure{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}},
			expected: []string{},
		},
		{
			name: "Empty Status fields does not return values",
			cluster: configv1.Infrastructure{ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Status: configv1.InfrastructureStatus{APIServerURL: "", APIServerInternalURL: ""},
			},
			expected: []string{},
		},
		{
			name: "Values are captured and returned properly",
			cluster: configv1.Infrastructure{ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Status: configv1.InfrastructureStatus{
					APIServerURL: "mock1", APIServerInternalURL: "mock2",
				}},
			expected: []string{"mock1", "mock2"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Given
			configClient := configfake.NewSimpleClientset().ConfigV1()
			_, err := configClient.Infrastructures().Create(context.Background(),
				&tc.cluster,
				metav1.CreateOptions{})
			assert.NoError(t, err)

			// When
			test, err := GetClusterAPIServerInfo(context.Background(), configClient)

			// Assert
			assert.NoError(t, err)
			assert.Equal(t, len(tc.expected), len(test))
			for i := 0; i < len(tc.expected); i++ {
				assert.Equal(t, tc.expected[i], test[i])
			}
		})
	}

}
