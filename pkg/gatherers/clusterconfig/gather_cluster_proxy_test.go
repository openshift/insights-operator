package clusterconfig

import (
	"context"
	"testing"

	v1 "github.com/openshift/api/config/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_GatherClusterProxy(t *testing.T) {
	// Unit Tests
	testCases := []struct {
		name       string
		proxy      *v1.Proxy
		result     []record.Record
		errorCount int
	}{
		{
			name:  "Retrieving proxy returns record of that proxy and no errors",
			proxy: &v1.Proxy{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}},
			result: []record.Record{
				{
					Name: "config/proxy",
					Item: record.ResourceMarshaller{
						Resource: &v1.Proxy{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}},
					},
				},
			},
			errorCount: 0,
		},
		{
			name:       "Retrieving no proxy returns no error/no record",
			proxy:      &v1.Proxy{},
			result:     nil,
			errorCount: 0,
		},
		{
			name: "Proxy status HTTPPROXY returns obfuscated string instead real value",
			proxy: &v1.Proxy{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Status:     v1.ProxyStatus{HTTPProxy: "0.0.0.0:8443"},
			},
			result: []record.Record{
				{
					Name: "config/proxy",
					Item: record.ResourceMarshaller{
						Resource: &v1.Proxy{
							ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
							Status:     v1.ProxyStatus{HTTPProxy: "x.x.x.x:xxxx"},
						},
					},
				},
			},
			errorCount: 0,
		},
		{
			name: "Proxy spec HTTPPROXY returns obfuscated string instead real value",
			proxy: &v1.Proxy{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec:       v1.ProxySpec{HTTPProxy: "0.0.0.0:8443"},
			},
			result: []record.Record{
				{
					Name: "config/proxy",
					Item: record.ResourceMarshaller{
						Resource: &v1.Proxy{
							ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
							Spec:       v1.ProxySpec{HTTPProxy: "x.x.x.x:xxxx"},
						},
					},
				},
			},
			errorCount: 0,
		},
	}

	for _, testCase := range testCases {
		tc := testCase
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Given
			configClient := configfake.NewSimpleClientset(tc.proxy)

			// When
			test, errs := gatherClusterProxy(context.Background(), configClient.ConfigV1())

			// Assert
			assert.Equal(t, tc.result, test)
			assert.Len(t, errs, tc.errorCount)
		})
	}
}
