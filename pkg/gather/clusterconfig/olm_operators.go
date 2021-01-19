package clusterconfig

import (
	"context"
	"encoding/json"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
)

type olmOperator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// GatherOLMOperators collects list of all names (including version) of installed OLM operators.
//
// See: docs/insights-archive-sample/config/olm_operators
// Location of in archive: config/olm_operators
// Id in config: olm_operators
func GatherOLMOperators(g *Gatherer, c chan<- gatherResult) {
	defer close(c)
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}
	records, errors := gatherOLMOperators(g.ctx, dynamicClient)
	c <- gatherResult{records, errors}
}

func gatherOLMOperators(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	gvr := schema.GroupVersionResource{Group: "operators.coreos.com", Version: "v1", Resource: "operators"}
	olmOperators, err := dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	var refs []interface{}
	olms := []olmOperator{}
	for _, i := range olmOperators.Items {
		err := parseJSONQuery(i.Object, "status.components.refs", &refs)
		if err != nil {
			klog.Errorf("Cannot find \"status.components.refs\" in %s definition: %v", i.GetName(), err)
			continue
		}
		for _, r := range refs {
			ver := readVersionFromRefs(r)
			if ver == "" {
				continue
			}
			olmO := olmOperator{
				Name:    i.GetName(),
				Version: ver,
			}
			if isInArray(olmO, olms) {
				continue
			}
			olms = append(olms, olmO)
		}
	}
	if len(olms) == 0 {
		return nil, nil
	}
	r := record.Record{
		Name: "config/olm_operators",
		Item: OlmOperatorAnonymizer{operators: olms},
	}
	return []record.Record{r}, nil
}

func isInArray(o olmOperator, a []olmOperator) bool {
	for _, op := range a {
		if o == op {
			return true
		}
	}
	return false
}

func readVersionFromRefs(r interface{}) string {
	refMap, ok := r.(map[string]interface{})
	if !ok {
		klog.Errorf("Cannot convert %s to map[string]interface{}", r)
		return ""
	}
	// version is part of the name of ClusterServiceVersion
	if refMap["kind"] == "ClusterServiceVersion" {
		name := refMap["name"].(string)
		nameVer := strings.SplitN(name, ".", 2)
		return nameVer[1]
	}
	return ""
}

// OlmOperatorAnonymizer implements HostSubnet serialization
type OlmOperatorAnonymizer struct{ operators []olmOperator }

// Marshal implements OlmOperator serialization
func (a OlmOperatorAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return json.Marshal(a.operators)
}

// GetExtension returns extension for OlmOperator object
func (a OlmOperatorAnonymizer) GetExtension() string {
	return "json"
}
