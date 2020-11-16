package clusterconfig

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestGatherStatefulSet(t *testing.T) {
	testSet := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-statefulset",
			Namespace: "openshift-test",
		},
	}
	client := kubefake.NewSimpleClientset()
	_, err := client.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "openshift-test"}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake namespace", err)
	}
	_, err = client.AppsV1().StatefulSets("openshift-test").Create(context.Background(), &testSet, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake statefulset", err)
	}
	records, errs := gatherStatefulSets(context.Background(), client.CoreV1(), client.AppsV1())
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}

	item, _ := records[0].Item.Marshal(context.TODO())
	var gatheredStatefulSet appsv1.StatefulSet
	_, _, err = appsV1Serializer.Decode(item, nil, &gatheredStatefulSet)
	if err != nil {
		t.Fatalf("failed to decode object: %v", err)
	}
	if gatheredStatefulSet.Name != "test-statefulset" {
		t.Fatalf("unexpected statefulset name %s", gatheredStatefulSet.Name)
	}

}
