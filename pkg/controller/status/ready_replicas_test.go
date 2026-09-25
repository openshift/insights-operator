package status

import (
	"context"
	"testing"

	operatorv1 "github.com/openshift/api/operator/v1"
	operatorfake "github.com/openshift/client-go/operator/clientset/versioned/fake"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestSyncReadyReplicas(t *testing.T) {
	const namespace = "openshift-insights"

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      insightsOperatorDeploymentName,
			Namespace: namespace,
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      1,
			ReadyReplicas: 1,
		},
	}
	insightsOperator := &operatorv1.InsightsOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name: insightsOperatorCRName,
		},
		Status: operatorv1.InsightsOperatorStatus{
			OperatorStatus: operatorv1.OperatorStatus{
				ReadyReplicas: 0,
				Version:       "5.0.0-rc.2",
			},
		},
	}

	kubeClient := kubefake.NewClientset(deploy)
	operatorClient := operatorfake.NewClientset(insightsOperator)

	var statusUpdates int
	operatorClient.PrependReactor("update", "insightsoperators", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "status" {
			statusUpdates++
		}
		return false, nil, nil
	})

	ctrl := &Controller{
		namespace:      namespace,
		kubeClient:     kubeClient,
		operatorClient: operatorClient.OperatorV1(),
	}

	ctx := context.Background()
	ctrl.syncReadyReplicas(ctx)

	updated, err := operatorClient.OperatorV1().InsightsOperators().Get(ctx, insightsOperatorCRName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get InsightsOperator: %v", err)
	}
	if updated.Status.ReadyReplicas != 1 {
		t.Fatalf("readyReplicas = %d, want 1", updated.Status.ReadyReplicas)
	}
	if updated.Status.Version != "5.0.0-rc.2" {
		t.Fatalf("version = %q, want other status fields preserved", updated.Status.Version)
	}
	if statusUpdates != 1 {
		t.Fatalf("status updates = %d, want 1", statusUpdates)
	}

	ctrl.syncReadyReplicas(ctx)
	if statusUpdates != 1 {
		t.Fatalf("second sync wrote status again, updates = %d", statusUpdates)
	}
}

func TestSyncReadyReplicasSkipsMissingClients(t *testing.T) {
	ctrl := &Controller{namespace: "openshift-insights"}
	ctrl.syncReadyReplicas(context.Background())
}
