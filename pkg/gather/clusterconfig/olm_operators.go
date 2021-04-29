package clusterconfig

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
)

var (
	operatorGVR              = schema.GroupVersionResource{Group: "operators.coreos.com", Version: "v1", Resource: "operators"}
	clusterServiceVersionGVR = schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "clusterserviceversions"}
)

type olmOperator struct {
	Name        string        `json:"name"`
	DisplayName string        `json:"displayName"`
	Version     string        `json:"version"`
	Conditions  []interface{} `json:"csv_conditions"`
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
// * See: docs/insights-archive-sample/config/olm_operators
// * Location of in archive: config/olm_operators
// * Id in config: olm_operators
// * Since versions:
//   * 4.7+
func (g *Gatherer) GatherOLMOperators(ctx context.Context) ([]record.Record, []error) {
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherOLMOperators(ctx, dynamicClient)
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
	errs := []error{}
	for _, i := range olmOperators.Items {
		newOlm := olmOperator{
			Name: i.GetName(),
		}
		err := utils.ParseJSONQuery(i.Object, "status.components.refs", &refs)
		if err != nil {
			// if no references are found then add an error and OLM operator with only name and continue
			errs = append(errs, fmt.Errorf("cannot find \"status.components.refs\" in %s definition: %v", i.GetName(), err))
			olms = append(olms, newOlm)
			continue
		}
		for _, r := range refs {
			csvRef, err := findCSVRefInRefs(r)
			if err != nil {
				errs = append(errs, err)
				olms = append(olms, newOlm)
				continue
			}
			// CSV reference can still be nil
			if csvRef == nil {
				continue
			}
			newOlm.Version = csvRef.Version

			name, conditions, err := getCSVAndParse(ctx, dynamicClient, csvRef)
			if err != nil {
				// append the error and the OLM data we already have and continue
				errs = append(errs, err)
				olms = append(olms, newOlm)
				continue
			}
			newOlm.DisplayName = name
			newOlm.Conditions = conditions

			if isInArray(newOlm, olms) {
				continue
			}
			olms = append(olms, newOlm)
		}
	}
	if len(olms) == 0 {
		return nil, nil
	}
	r := record.Record{
		Name: "config/olm_operators",
		Item: record.JSONMarshaller{Object: olms},
	}
	if len(errs) != 0 {
		return []record.Record{r}, errs
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

// getCSVAndParse gets full CSV definition from csvRef and tries to parse the definition
func getCSVAndParse(ctx context.Context,
	dynamicClient dynamic.Interface,
	csvRef *csvRef) (name string, conditions []interface{}, err error) {
	csv, err := getCsvFromRef(ctx, dynamicClient, csvRef)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get %s ClusterServiceVersion: %v", csvRef.Name, err)
	}
	name, conditions, err = parseCsv(csv)

	if err != nil {
		return "", nil, fmt.Errorf("cannot read %s ClusterServiceVersion attributes: %v", csvRef.Name, err)
	}

	return name, conditions, nil
}

// findCSVRefInRefs tries to find ClusterServiceVersion reference in the references
// and parse the ClusterServiceVersion if successful.
// It can return nil with no error if the CSV was not found
func findCSVRefInRefs(r interface{}) (*csvRef, error) {
	refMap, ok := r.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("cannot convert %s to map[string]interface{}", r)
	}
	// version is part of the name of ClusterServiceVersion
	if refMap["kind"] == "ClusterServiceVersion" {
		name := refMap["name"].(string)
		if !strings.Contains(name, ".") {
			return nil, fmt.Errorf("clusterserviceversion \"%s\" probably doesn't include version", name)
		}
		nameVer := strings.SplitN(name, ".", 2)
		csvRef := &csvRef{
			Name:      name,
			Namespace: refMap["namespace"].(string),
			Version:   nameVer[1],
		}
		return csvRef, nil
	}
	return nil, nil
}

func getCsvFromRef(ctx context.Context, dynamicClient dynamic.Interface, csvRef *csvRef) (map[string]interface{}, error) {
	csv, err := dynamicClient.Resource(clusterServiceVersionGVR).Namespace(csvRef.Namespace).Get(ctx, csvRef.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return csv.Object, nil
}

// parseCsv tries to parse "status.conditions" and "spec.displayName" from the input map.
// Returns an error if any of the values cannot be parsed.
func parseCsv(csv map[string]interface{}) (name string, conditions []interface{}, err error) {
	err = utils.ParseJSONQuery(csv, "status.conditions", &conditions)
	if err != nil {
		return "", nil, err
	}
	err = utils.ParseJSONQuery(csv, "spec.displayName", &name)
	if err != nil {
		return "", nil, err
	}

	return name, conditions, nil
}
