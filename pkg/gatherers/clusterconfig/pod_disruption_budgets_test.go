package clusterconfig

import (
	"context"
	"testing"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func Test_PodDisruptionBudgets_Gather(t *testing.T) {
	tests := []struct {
		name string
		// namespace -> pdb
		pdbsToNamespace map[string]string
		// pdb -> minAvailable
		minAvailableToPdb map[string]int
		expCount          int
	}{
		{
			name:              "no PDB",
			pdbsToNamespace:   nil,
			minAvailableToPdb: nil,
			expCount:          0,
		},
		{
			name: "one openshift PDB",
			pdbsToNamespace: map[string]string{
				"openshift-test": "testT",
			},
			minAvailableToPdb: map[string]int{
				"testT": 1,
			},
			expCount: 1,
		},
		{
			name: "one random PDB",
			pdbsToNamespace: map[string]string{
				"random": "testR",
			},
			minAvailableToPdb: map[string]int{
				"testR": 1,
			},
			expCount: 0,
		},
		{
			name: "one openshift, one random PDB",
			pdbsToNamespace: map[string]string{
				"openshift-test": "testT",
				"random":         "testR",
			},
			minAvailableToPdb: map[string]int{
				"testT": 1,
				"testR": 1,
			},
			expCount: 1,
		},
		{
			name: "multiple openshift PDBs",
			pdbsToNamespace: map[string]string{
				"openshift-test":    "testT",
				"openshift-default": "testD",
			},
			minAvailableToPdb: map[string]int{
				"testT": 1,
				"testD": 2,
			},
			expCount: 2,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			coreClient := kubefake.NewSimpleClientset()

			for namespace, name := range test.pdbsToNamespace {
				_, err := coreClient.PolicyV1().
					PodDisruptionBudgets(namespace).
					Create(context.Background(), &policyv1.PodDisruptionBudget{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace,
							Name:      name,
						},
						Spec: policyv1.PodDisruptionBudgetSpec{
							MinAvailable: &intstr.IntOrString{IntVal: int32(test.minAvailableToPdb[name])},
						},
					}, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("unable to create fake pdbs: %v", err)
				}
			}
			ctx := context.Background()
			records, errs := gatherPodDisruptionBudgets(ctx, coreClient.PolicyV1())
			assert.Emptyf(t, errs, "unexpected errors: %#v", errs)
			assert.Equal(t, test.expCount, len(records))
			if len(records) > 1 {
				for i := range records {
					pdba, ok := records[i].Item.(record.ResourceMarshaller).Resource.(*policyv1.PodDisruptionBudget)
					if !ok {
						t.Fatal("pdb item has invalid type")
					}

					name := pdba.ObjectMeta.Name
					namespace := pdba.ObjectMeta.Namespace
					assert.Equal(t, test.pdbsToNamespace[namespace], name)
					minAvailable := pdba.Spec.MinAvailable.StrVal
					if pdba.Spec.MinAvailable.IntValue() != test.minAvailableToPdb[name] {
						t.Fatalf("pdb item has mismatched MinAvailable value, %q != %q", test.minAvailableToPdb[name], minAvailable)
					}
				}
			}
		})
	}
}
