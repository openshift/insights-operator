package clusterconfig

import (
	"context"
	"testing"

	"github.com/openshift/insights-operator/pkg/record"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func Test_PodDisruptionBudgets_Gather(t *testing.T) {
	coreClient := kubefake.NewSimpleClientset()

	fakeNamespace := "fake-namespace"

	// name -> MinAvailabel
	fakePDBs := map[string]string{
		"pdb-four":  "4",
		"pdb-eight": "8",
		"pdb-ten":   "10",
	}
	for name, minAvailable := range fakePDBs {
		_, err := coreClient.PolicyV1().
			PodDisruptionBudgets(fakeNamespace).
			Create(context.Background(), &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: fakeNamespace,
					Name:      name,
				},
				Spec: policyv1.PodDisruptionBudgetSpec{
					MinAvailable: &intstr.IntOrString{StrVal: minAvailable},
				},
			}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("unable to create fake pdbs: %v", err)
		}
	}
	ctx := context.Background()
	records, errs := gatherPodDisruptionBudgets(ctx, coreClient.PolicyV1())
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}
	if len(records) != len(fakePDBs) {
		t.Fatalf("unexpected number of records gathered: %d (expected %d)", len(records), len(fakePDBs))
	}
	pdba, ok := records[0].Item.(record.ResourceMarshaller).Resource.(*policyv1.PodDisruptionBudget)
	if !ok {
		t.Fatal("pdb item has invalid type")
	}
	name := pdba.ObjectMeta.Name
	minAvailable := pdba.Spec.MinAvailable.StrVal
	if pdba.Spec.MinAvailable.StrVal != fakePDBs[name] {
		t.Fatalf("pdb item has mismatched MinAvailable value, %q != %q", fakePDBs[name], minAvailable)
	}
}
