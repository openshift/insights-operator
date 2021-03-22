package clusterconfig

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
)

var (
	operatorGVR              = schema.GroupVersionResource{Group: "operators.coreos.com", Version: "v1", Resource: "operators"}
	clusterServiceVersionGVR = schema.GroupVersionResource{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "clusterserviceversions"}
)

type olmOperator struct {
	Name       string        `json:"name"`
	Version    string        `json:"version"`
	Conditions []interface{} `json:"csv_conditions"`
}

// ClusterServiceVersion helper struct
type csvRef struct {
	Name      string
	Namespace string
	Version   string
}

// GatherOLMOperators collects list of installed OLM operators.
// Each OLM operator (in the list) contains following data:
// - OLM operator name
// - OLM operator version
// - related ClusterServiceVersion conditions
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
	olmOperators, err := dynamicClient.Resource(operatorGVR).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	var refs []interface{}
	olms := []olmOperator{}
	for _, i := range olmOperators.Items {
		err := utils.ParseJSONQuery(i.Object, "status.components.refs", &refs)
		if err != nil {
			klog.Errorf("Cannot find \"status.components.refs\" in %s definition: %v", i.GetName(), err)
			continue
		}
		for _, r := range refs {
			csvRef := getCSVRefFromRefs(r)
			if csvRef == nil {
				continue
			}
			conditions, err := getCSVConditions(ctx, dynamicClient, csvRef)
			if err != nil {
				klog.Errorf("failed to get %s conditions: %v", csvRef.Name, err)
				continue
			}
			olmO := olmOperator{
				Name:       i.GetName(),
				Version:    csvRef.Version,
				Conditions: conditions,
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
		Item: record.JSONMarshaller{Object: olms},
	}
	return []record.Record{r}, nil
}

func isInArray(o olmOperator, a []olmOperator) bool {
	for _, op := range a {
		if o.Name == op.Name && o.Version == op.Version {
			return true
		}
	}
	return false
}

func getCSVRefFromRefs(r interface{}) *csvRef {
	refMap, ok := r.(map[string]interface{})
	if !ok {
		klog.Errorf("Cannot convert %s to map[string]interface{}", r)
		return nil
	}
	// version is part of the name of ClusterServiceVersion
	if refMap["kind"] == "ClusterServiceVersion" {
		name := refMap["name"].(string)
		nameVer := strings.SplitN(name, ".", 2)
		csvRef := &csvRef{
			Name:      name,
			Namespace: refMap["namespace"].(string),
			Version:   nameVer[1],
		}
		return csvRef
	}
	return nil
}

func getCSVConditions(ctx context.Context, dynamicClient dynamic.Interface, csvRef *csvRef) ([]interface{}, error) {
	csv, err := dynamicClient.Resource(clusterServiceVersionGVR).Namespace(csvRef.Namespace).Get(ctx, csvRef.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	var conditions []interface{}
	err = utils.ParseJSONQuery(csv.Object, "status.conditions", &conditions)
	if err != nil {
		return nil, err
	}
	return conditions, nil
}
