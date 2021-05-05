package clusterconfig

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/openshift/insights-operator/pkg/utils"
)

//nolint: funlen, lll, gocyclo
func Test_InstallPlans_Gather(t *testing.T) {
	tests := []struct {
		name      string
		testfiles []string
		limit     int
		exp       string
	}{
		{
			name:      "one installplan",
			testfiles: []string{"testdata/installplan.yaml"},
			exp: `{"items":[{"count":1,"csv":"lib-bucket-provisioner.v2.0.0","name":"install-","ns":"openshift-operators"}],` +
				`"stats":{"TOTAL_COUNT":1,"TOTAL_NONUNIQ_COUNT":1}}`,
		},
		{
			name:      "two are same to keep ordering and one is different",
			testfiles: []string{"testdata/installplan.yaml", "testdata/installplan2.yaml", "testdata/installplan_openshift.yaml"},
			exp:       `{"items":[{"count":2,"csv":"lib-bucket-provisioner.v2.0.0","name":"install-","ns":"openshift-operators"},{"count":1,"csv":"3scale-community-operator.v0.5.1","name":"install-","ns":"openshift"}],"stats":{"TOTAL_COUNT":3,"TOTAL_NONUNIQ_COUNT":2}}`,
		},
		{
			name:      "two similar installplans",
			testfiles: []string{"testdata/installplan.yaml", "testdata/installplan2.yaml"},
			exp: `{"items":[{"count":2,"csv":"lib-bucket-provisioner.v2.0.0","name":"install-","ns":"openshift-operators"}],` +
				`"stats":{"TOTAL_COUNT":2,"TOTAL_NONUNIQ_COUNT":1}}`,
		},
		{
			name:      "test marshaller with limit to 1 item",
			testfiles: []string{"testdata/installplan.yaml", "testdata/installplan2.yaml", "testdata/installplan_openshift.yaml"},
			limit:     1,
			exp:       `{"items":[{"count":2,"csv":"lib-bucket-provisioner.v2.0.0","name":"install-","ns":"openshift-operators"}],"stats":{"TOTAL_COUNT":3,"TOTAL_NONUNIQ_COUNT":2}}`,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var client *dynamicfake.FakeDynamicClient
			coreClient := kubefake.NewSimpleClientset()
			for _, file := range test.testfiles {
				f, err := os.Open(file)
				if err != nil {
					t.Fatal("test failed to read installplan data", err)
				}
				defer f.Close()
				installplancontent, err := ioutil.ReadAll(f)
				if err != nil {
					t.Fatal("error reading test data file", err)
				}

				decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
				installplan := &unstructured.Unstructured{}

				_, _, err = decUnstructured.Decode(installplancontent, nil, installplan)
				if err != nil {
					t.Fatal("unable to decode", err)
				}
				gv, _ := schema.ParseGroupVersion(installplan.GetAPIVersion())
				gvr := schema.GroupVersionResource{Version: gv.Version, Group: gv.Group, Resource: "installplans"}
				var ns string
				err = utils.ParseJSONQuery(installplan.Object, "metadata.namespace", &ns)
				if err != nil {
					t.Fatal("unable to read ns ", err)
				}
				_, err = coreClient.CoreV1().Namespaces().Get(context.Background(), ns, metav1.GetOptions{})
				if errors.IsNotFound(err) {
					_, err = coreClient.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}, metav1.CreateOptions{})
				}
				if err != nil {
					t.Fatal("unable to create ns fake ", err)
				}
				if client == nil {
					client = dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
						gvr: "InstallPlansList",
					})
				}
				_, err = client.Resource(gvr).Namespace(ns).Create(context.Background(), installplan, metav1.CreateOptions{})
				if err != nil {
					t.Fatal("unable to create installplan fake ", err)
				}
			}
			ctx := context.Background()
			records, errs := gatherInstallPlans(ctx, client, coreClient.CoreV1())
			if len(errs) > 0 {
				t.Errorf("unexpected errors: %#v", errs)
				return
			}
			if len(records) != 1 {
				t.Fatalf("unexpected number or records %d", len(records))
			}
			m, ok := records[0].Item.(InstallPlanAnonymizer)
			if !ok {
				t.Fatalf("returned item is not of type InstallPlanAnonymizer")
			}
			if test.limit != 0 {
				// copy to new anonymizer with limited max
				m = InstallPlanAnonymizer{limit: 1, total: m.total, v: m.v}
			}
			b, _ := m.Marshal(context.Background())
			sb := string(b)
			if sb != test.exp {
				t.Fatalf("unexpected installplan exp: %s got: %s", test.exp, sb)
			}
		})
	}
}
