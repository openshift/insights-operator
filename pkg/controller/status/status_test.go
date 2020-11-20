package status

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"

	configv1 "github.com/openshift/api/config/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/utils"
	kubeclientfake "k8s.io/client-go/kubernetes/fake"
)

func TestSaveInitialStart(t *testing.T) {

	tests := []struct {
		name                     string
		clusterOperator          *configv1.ClusterOperator
		expErr                   error
		initialRun               bool
		expectedSafeInitialStart bool
	}{
		{
			name:                     "Non-initial run is has upload delayed",
			initialRun:               false,
			expectedSafeInitialStart: false,
		},
		{
			name:                     "Initial run with not existing Insights operator is not delayed",
			initialRun:               true,
			clusterOperator:          nil,
			expectedSafeInitialStart: true,
		},
		{
			name:       "Initial run with existing Insights operator which is degraded is delayed",
			initialRun: true,
			clusterOperator: &configv1.ClusterOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: "insights",
				},
				Status: configv1.ClusterOperatorStatus{Conditions: []configv1.ClusterOperatorStatusCondition{
					{Type: configv1.OperatorDegraded, Status: configv1.ConditionTrue},
				}},
			},
			expectedSafeInitialStart: false,
		},
		{
			name:       "Initial run with existing Insights operator which is not degraded not delayed",
			initialRun: true,
			clusterOperator: &configv1.ClusterOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: "insights",
				},
				Status: configv1.ClusterOperatorStatus{Conditions: []configv1.ClusterOperatorStatusCondition{
					{Type: configv1.OperatorDegraded, Status: configv1.ConditionFalse},
				}},
			},
			expectedSafeInitialStart: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			klog.SetOutput(utils.NewTestLog(t).Writer())
			operators := []runtime.Object{}
			if tt.clusterOperator != nil {
				operators = append(operators, tt.clusterOperator)
			}
			kubeclientsetclient := kubeclientfake.NewSimpleClientset()

			client := configfake.NewSimpleClientset(operators...)
			ctrl := &Controller{name: "insights", client: client.ConfigV1(), configurator: configobserver.New(config.Controller{Report: true}, kubeclientsetclient)}

			err := ctrl.updateStatus(context.Background(), tt.initialRun)
			if err != tt.expErr {
				t.Fatalf("updateStatus returned unexpected error: %s Expected %s", err, tt.expErr)
			}
		})
	}
}
