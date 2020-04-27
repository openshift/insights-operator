package clusterconfig

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"

	configv1 "github.com/openshift/api/config/v1"
	clientset "github.com/openshift/client-go/config/clientset/versioned"
	openshiftclientsetfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/kubernetes"
	clsetfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/testing"
	"k8s.io/client-go/util/flowcontrol"
)

func ExampleMostRecentMetrics() (string, error) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()

	c := http.DefaultClient
	u, _ := url.Parse(ts.URL)
	re := rest.NewRequestWithClient(u, "", rest.ClientContentConfig{}, c).Verb("get")

	r := mockRest{GetMock: re}
	g := &Gatherer{metricsClient: r}
	d, errs := GatherMostRecentMetrics(g)()
	if len(errs) > 0 {
		return "", errs[0]
	}
	b, err := json.Marshal(d)
	return string(b), err
}

func ExampleClusterOperators() (string, error) {
	kube := openshiftClientResponder{}

	kube.Fake.AddReactor("list", "clusteroperators",
		func(action testing.Action) (handled bool, ret runtime.Object, err error) {
			sv := &configv1.ClusterOperatorList{Items: []configv1.ClusterOperator{
				configv1.ClusterOperator{Status: configv1.ClusterOperatorStatus{
					Conditions: []configv1.ClusterOperatorStatusCondition{
						configv1.ClusterOperatorStatusCondition{Type: configv1.OperatorDegraded},
					}},
				}}}
			return true, sv, nil
		})

	g := &Gatherer{client: kube.ConfigV1()}
	d, errs := GatherClusterOperators(g)()
	if len(errs) > 0 {
		return "", errs[0]
	}
	b, err := json.Marshal(d)
	return string(b), err
}

func ExampleUnhealthyNodes() (string, error) {
	kube := kubeClientResponder{}

	kube.Fake.AddReactor("list", "nodes",
		func(action testing.Action) (handled bool, ret runtime.Object, err error) {
			sv := &corev1.NodeList{Items: []corev1.Node{
				corev1.Node{Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
					}},
				}}}
			return true, sv, nil
		})

	g := &Gatherer{coreClient: kube.CoreV1()}
	d, errs := GatherUnhealthyNodes(g)()
	if len(errs) > 0 {
		return "", errs[0]
	}
	b, err := json.Marshal(d)
	return string(b), err
}

type mockRest struct {
	APIVersionMock schema.GroupVersion
	DeleteMock     *rest.Request
	PostMock       *rest.Request
	PutMock        *rest.Request
	GetMock        *rest.Request
	PatchMock      *rest.Request
	VerbMock       *rest.Request
}

func (m mockRest) GetRateLimiter() flowcontrol.RateLimiter {
	return nil
}
func (m mockRest) Verb(verb string) *rest.Request {
	return m.VerbMock
}
func (m mockRest) Post() *rest.Request {
	return m.PostMock
}
func (m mockRest) Put() *rest.Request {
	return m.PutMock
}
func (m mockRest) Patch(pt types.PatchType) *rest.Request {
	return m.PatchMock
}
func (m mockRest) Get() *rest.Request {
	return m.GetMock
}
func (m mockRest) Delete() *rest.Request {
	return m.DeleteMock
}
func (m mockRest) APIVersion() schema.GroupVersion {
	return m.APIVersionMock
}

type openshiftClientResponder struct {
	openshiftclientsetfake.Clientset
}

type kubeClientResponder struct {
	clsetfake.Clientset
}

var (
	_ clientset.Interface  = (*openshiftClientResponder)(nil)
	_ kubernetes.Interface = (*kubeClientResponder)(nil)
)
