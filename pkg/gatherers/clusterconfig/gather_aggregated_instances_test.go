package clusterconfig

import (
	"context"
	"testing"

	"github.com/openshift/insights-operator/pkg/record"
	v1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/prometheus-operator/prometheus-operator/pkg/client/versioned/fake"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Test_GatherAggregatedInstances provides unit tests for the correct output file structure
func Test_GatherAggregatedInstances(t *testing.T) {
	testCases := []struct {
		name      string
		proms     []*v1.Prometheus
		alertMgrs []*v1.Alertmanager
		expected  []record.Record
	}{
		{
			name: "The function returns the name of the Prometheus instance in the correct field",
			proms: []*v1.Prometheus{
				{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-namespace"}},
			},
			expected: []record.Record{{
				Name: "aggregated/custom_prometheuses_alertmanagers",
				Item: record.JSONMarshaller{Object: aggregatedInstances{
					Prometheuses: []string{"test"}, Alertmanagers: []string{},
				}}},
			},
		}, {
			name: "The function returns the name of the AlertManager instance in the correct field",
			alertMgrs: []*v1.Alertmanager{
				{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-namespace"}},
			},
			expected: []record.Record{{
				Name: "aggregated/custom_prometheuses_alertmanagers",
				Item: record.JSONMarshaller{Object: aggregatedInstances{
					Alertmanagers: []string{"test"}, Prometheuses: []string{},
				}}},
			},
		}, {
			name: "The function returns the names of the mixed instances in the correct field",
			alertMgrs: []*v1.Alertmanager{
				{ObjectMeta: metav1.ObjectMeta{Name: "test-alertmanager", Namespace: "test-namespace"}},
			},
			proms: []*v1.Prometheus{
				{ObjectMeta: metav1.ObjectMeta{Name: "test-prometheus", Namespace: "test-namespace"}},
			},
			expected: []record.Record{{
				Name: "aggregated/custom_prometheuses_alertmanagers",
				Item: record.JSONMarshaller{Object: aggregatedInstances{
					Alertmanagers: []string{"test-alertmanager"}, Prometheuses: []string{"test-prometheus"},
				}}},
			},
		}, {
			name:      "The function returns an empty records file if no instances are found",
			alertMgrs: []*v1.Alertmanager{},
			proms:     []*v1.Prometheus{},
			expected: []record.Record{{
				Name: "aggregated/custom_prometheuses_alertmanagers",
				Item: record.JSONMarshaller{Object: aggregatedInstances{
					Alertmanagers: []string{}, Prometheuses: []string{},
				}}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Given
			clientset := fake.NewSimpleClientset()
			for _, am := range tc.alertMgrs {
				clientset.Tracker().Add(am)
			}
			for _, prom := range tc.proms {
				clientset.Tracker().Add(prom)
			}

			// When
			test, errs := aggregatedInstances{}.gather(context.Background(), clientset)

			// Assert
			assert.Empty(t, errs)
			assert.EqualValues(t, tc.expected, test)
		})
	}
}

// Test_getOutcastedAlertManagers provides unit tests for the namespace filtering logic of AlertManager instances
func Test_getOutcastedAlertManagers(t *testing.T) {
	testCases := []struct {
		name      string
		alertMgrs []*v1.Alertmanager
		expected  []string
	}{
		{
			name: "The function returns the name of the Prometheus outside the 'openshift-monitoring' namespace",
			alertMgrs: []*v1.Alertmanager{
				{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-namespace"}},
			},
			expected: []string{"test"},
		}, {
			name: "The function ignores the name of the Prometheus inside the 'openshift-monitoring' namespace",
			alertMgrs: []*v1.Alertmanager{
				{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "openshift-monitoring"}},
			},
			expected: []string{},
		}, {
			name: "The function returns only items outside of the namespace on a mixed response from client",
			alertMgrs: []*v1.Alertmanager{
				{ObjectMeta: metav1.ObjectMeta{Name: "test1", Namespace: "test-namespace"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ignore", Namespace: "openshift-monitoring"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "test2", Namespace: "test-namespace"}},
			},
			expected: []string{"test1", "test2"},
		}, {
			name:      "The function returns an empty slice if no instances are found",
			alertMgrs: []*v1.Alertmanager{},
			expected:  []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Given
			clientset := fake.NewSimpleClientset()
			for _, am := range tc.alertMgrs {
				clientset.Tracker().Add(am)
			}

			// When
			test, err := aggregatedInstances{}.getOutcastedAlertManagers(context.Background(), clientset)

			// Assert
			assert.NoError(t, err)
			assert.EqualValues(t, tc.expected, test)
		})
	}
}

// Test_getOutcastedPrometheuses provides unit tests for the namespace filtering logic of Prometheus instances
func Test_getOutcastedPrometheuses(t *testing.T) {
	testCases := []struct {
		name     string
		proms    []*v1.Prometheus
		expected []string
	}{
		{
			name: "The function returns the name of the Prometheus outside the 'openshift-monitoring' namespace",
			proms: []*v1.Prometheus{
				{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-namespace"}},
			},
			expected: []string{"test"},
		}, {
			name: "The function ignores the name of the Prometheus inside the 'openshift-monitoring' namespace",
			proms: []*v1.Prometheus{
				{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "openshift-monitoring"}},
			},
			expected: []string{},
		}, {
			name: "The function returns only items outside of the namespace on a mixed response from client",
			proms: []*v1.Prometheus{
				{ObjectMeta: metav1.ObjectMeta{Name: "test1", Namespace: "test-namespace"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ignore", Namespace: "openshift-monitoring"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "test2", Namespace: "test-namespace"}},
			},
			expected: []string{"test1", "test2"},
		}, {
			name:     "The function returns an empty slice if no instances are found",
			proms:    []*v1.Prometheus{},
			expected: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Given
			clientset := fake.NewSimpleClientset()
			for _, prom := range tc.proms {
				clientset.Tracker().Add(prom)
			}

			// When
			test, err := aggregatedInstances{}.getOutcastedPrometheuses(context.Background(), clientset)

			// Assert
			assert.NoError(t, err)
			assert.EqualValues(t, tc.expected, test)
		})
	}
}
