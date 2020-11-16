package clusterconfig

import (
	"context"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestGatherClusterOperator(t *testing.T) {
	testOperator := &configv1.ClusterOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-clusteroperator",
		},
	}
	configCS := configfake.NewSimpleClientset()
	_, err := configCS.ConfigV1().ClusterOperators().Create(context.Background(), testOperator, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake clusteroperator", err)
	}
	records, errs := gatherClusterOperators(context.Background(), configCS.ConfigV1(), kubefake.NewSimpleClientset().CoreV1(), configCS.Discovery(), dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()))
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}

	item, _ := records[0].Item.Marshal(context.TODO())
	var gatheredCO configv1.ClusterOperator
	_, _, err = openshiftSerializer.Decode(item, nil, &gatheredCO)
	if err != nil {
		t.Fatalf("failed to decode object: %v", err)
	}
	if gatheredCO.Name != "test-clusteroperator" {
		t.Fatalf("unexpected clusteroperator name %s", gatheredCO.Name)
	}

}
