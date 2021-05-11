package clusterconfig

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
)

// InstallPlansTopX is the Maximal number of Install plans by non-unique instances count
const InstallPlansTopX = 100

// GatherInstallPlans collects Top x InstallPlans from all openshift namespaces.
// Because InstallPlans have unique generated names, it groups them by namespace and the "template"
// for name generation from field generateName.
// It also collects Total number of all installplans and all non-unique installplans.
//
// The Operators-Framework api https://github.com/operator-framework/api/blob/master/pkg/operators/v1alpha1/installplan_types.go#L26
//
// * Location in archive: config/installplans/
// * Id in config: install_plans
// * Since versions:
//   * 4.5.33+
//   * 4.6.16+
//   * 4.7+
func (g *Gatherer) GatherInstallPlans(ctx context.Context) ([]record.Record, []error) {
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherInstallPlans(ctx, dynamicClient, gatherKubeClient.CoreV1())
}

func gatherInstallPlans(ctx context.Context,
	dynamicClient dynamic.Interface,
	coreClient corev1client.CoreV1Interface) ([]record.Record, []error) {
	var plansBatchLimit int64 = 500
	cont := ""
	recs := map[string]*collectedPlan{}
	total := 0
	opResource := schema.GroupVersionResource{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "installplans"}
	config, err := utils.GetAllNamespaces(ctx, coreClient)
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	// collect from all openshift* namespaces
	for i := range config.Items {
		if !strings.HasPrefix(config.Items[i].Name, "openshift") {
			continue
		}
		resInterface := dynamicClient.Resource(opResource).Namespace(config.Items[i].Name)
		for {
			u, err := resInterface.List(ctx, metav1.ListOptions{Limit: plansBatchLimit, Continue: cont})
			if errors.IsNotFound(err) {
				return nil, nil
			}
			if err != nil {
				return nil, []error{err}
			}
			jsonMap := u.UnstructuredContent()
			var items []interface{}
			err = failEarly(
				func() error { return utils.ParseJSONQuery(jsonMap, "metadata.continue?", &cont) },
				func() error { return utils.ParseJSONQuery(jsonMap, "items", &items) },
			)
			if err != nil {
				return nil, []error{err}
			}
			total += len(items)
			for _, item := range items {
				if errs := collectInstallPlan(recs, item); errs != nil {
					return nil, errs
				}
			}

			if cont == "" {
				break
			}
		}
	}

	return []record.Record{{Name: "config/installplans", Item: InstallPlanAnonymizer{v: recs, total: total}}}, nil
}

func collectInstallPlan(recs map[string]*collectedPlan, item interface{}) []error {
	// Get common prefix
	csv := "[NONE]"
	var clusterServiceVersionNames []interface{}
	var ns, genName string
	var itemMap map[string]interface{}
	var ok bool
	if itemMap, ok = item.(map[string]interface{}); !ok {
		return []error{fmt.Errorf("cannot cast item to map %v", item)}
	}

	err := failEarly(
		func() error {
			return utils.ParseJSONQuery(itemMap, "spec.clusterServiceVersionNames", &clusterServiceVersionNames)
		},
		func() error { return utils.ParseJSONQuery(itemMap, "metadata.namespace", &ns) },
		func() error { return utils.ParseJSONQuery(itemMap, "metadata.generateName", &genName) },
	)
	if err != nil {
		return []error{err}
	}
	if len(clusterServiceVersionNames) > 0 {
		// ignoring non string
		csv, _ = clusterServiceVersionNames[0].(string)
	}

	key := fmt.Sprintf("%s.%s.%s", ns, genName, csv)
	m, ok := recs[key]
	if !ok {
		recs[key] = &collectedPlan{Namespace: ns, Name: genName, CSV: csv, Count: 1}
	} else {
		m.Count++
	}
	return nil
}

func failEarly(fns ...func() error) error {
	for _, f := range fns {
		err := f()
		if err != nil {
			return err
		}
	}
	return nil
}

type collectedPlan struct {
	Namespace string
	Name      string
	CSV       string
	Count     int
}

// InstallPlanAnonymizer implements serialization of top x installplans
type InstallPlanAnonymizer struct {
	v     map[string]*collectedPlan
	total int
	limit int
}

// Marshal implements serialization of InstallPlan
func (a InstallPlanAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	if a.limit == 0 {
		a.limit = InstallPlansTopX
	}

	var cnts []int
	for _, v := range a.v {
		cnts = append(cnts, v.Count)
	}

	sort.Sort(sort.Reverse(sort.IntSlice(cnts)))
	countLimit := -1
	if len(cnts) > a.limit && a.limit > 0 {
		// nth plan is on n-1th position
		countLimit = cnts[a.limit-1]
	}

	// Creates map for marshal
	sr := map[string]interface{}{}
	st := map[string]int{}
	st["TOTAL_COUNT"] = a.total
	st["TOTAL_NONUNIQ_COUNT"] = len(a.v)
	sr["stats"] = st
	uls := 0

	var it []interface{}
	for _, v := range a.v {
		if v.Count >= countLimit {
			kvp := map[string]interface{}{}
			kvp["ns"] = v.Namespace
			kvp["name"] = v.Name
			kvp["csv"] = v.CSV
			kvp["count"] = v.Count
			it = append(it, kvp)
			uls++
		}
		if uls >= a.limit {
			break
		}
	}
	sort.SliceStable(it, func(i, j int) bool {
		return it[i].(map[string]interface{})["count"].(int) > it[j].(map[string]interface{})["count"].(int)
	})
	sr["items"] = it

	return json.Marshal(sr)
}

// GetExtension returns extension for anonymized openshift objects
func (a InstallPlanAnonymizer) GetExtension() string {
	return jsonExtension
}
