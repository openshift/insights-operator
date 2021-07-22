package clusterconfig

import (
	"context"
	"fmt"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
)

const (
	// Log compression ratio is defining a multiplier for uncompressed logs
	// recorder would refuse to write files larger than MaxLogSize, so GatherClusterOperators
	// has to limit the expected size of the buffer for logs
	logCompressionRatio = 2
)

type clusterOperatorResource struct {
	APIVersion string      `json:"apiVersion"`
	Kind       string      `json:"kind"`
	Name       string      `json:"name"`
	Spec       interface{} `json:"spec"`
}

// GatherClusterOperators collects all the ClusterOperators definitions and their resources.
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/clusteroperator.go#L62
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#clusteroperatorlist-v1config-openshift-io
//
// * Location of operators related resources: config/clusteroperator/{group}/{kind}/{name}
// * Location of operators in archive: config/clusteroperator/
// * Location of operators related resources in older versions: config/clusteroperator/{kind}-{name}
// * See: docs/insights-archive-sample/config/clusteroperator
// * Id in config: operators
// * Spec config for CO resources since versions:
//   * 4.6.16+
//   * 4.7+
func (g *Gatherer) GatherClusterOperators(ctx context.Context) ([]record.Record, []error) {
	gatherConfigClient, err := configv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	records, err := gatherClusterOperators(ctx, gatherConfigClient, discoveryClient, dynamicClient)
	if err != nil {
		return records, []error{err}
	}

	return records, nil
}

// gatherClusterOperators collects cluster operators
func gatherClusterOperators(ctx context.Context,
	configClient configv1client.ConfigV1Interface,
	discoveryClient discovery.DiscoveryInterface,
	dynamicClient dynamic.Interface) ([]record.Record, error) {
	config, err := configClient.ClusterOperators().List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// collect the cluster operators reports
	records := clusterOperatorsRecords(ctx, config.Items, dynamicClient, discoveryClient)
	return records, nil
}

// clusterOperatorsRecords generates the cluster operator records
func clusterOperatorsRecords(ctx context.Context,
	items []configv1.ClusterOperator,
	dynamicClient dynamic.Interface,
	discoveryClient discovery.DiscoveryInterface) []record.Record {
	resVer, _ := getOperatorResourcesVersions(discoveryClient)
	records := make([]record.Record, 0, len(items))

	for idx := range items {
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/clusteroperator/%s", items[idx].Name),
			Item: record.ResourceMarshaller{Resource: &items[idx]},
		})
		if resVer == nil {
			continue
		}

		relRes := collectClusterOperatorResources(ctx, dynamicClient, items[idx], resVer)
		for _, rr := range relRes {
			// imageregistry resources (config, pruner) are gathered in image_registries.go, image_pruners.go
			if strings.Contains(rr.APIVersion, "imageregistry") {
				continue
			}
			gv, err := schema.ParseGroupVersion(rr.APIVersion)
			if err != nil {
				klog.Warningf("Unable to parse group version %s: %s", rr.APIVersion, err)
			}
			records = append(records, record.Record{
				Name: fmt.Sprintf("config/clusteroperator/%s/%s/%s", gv.Group, strings.ToLower(rr.Kind), rr.Name),
				Item: record.JSONMarshaller{Object: rr},
			})
		}
	}

	return records
}

// collectClusterOperatorResources list all cluster operator resources
func collectClusterOperatorResources(ctx context.Context,
	dynamicClient dynamic.Interface,
	co configv1.ClusterOperator, //nolint: gocritic
	resVer map[string][]string) []clusterOperatorResource {
	var relObj []configv1.ObjectReference
	for _, ro := range co.Status.RelatedObjects {
		if strings.Contains(ro.Group, "operator.openshift.io") {
			relObj = append(relObj, ro)
		}
	}
	if len(relObj) == 0 {
		return nil
	}
	var res []clusterOperatorResource
	for _, ro := range relObj {
		key := fmt.Sprintf("%s-%s", ro.Group, strings.ToLower(ro.Resource))
		versions := resVer[key]
		for _, v := range versions {
			gvr := schema.GroupVersionResource{Group: ro.Group, Version: v, Resource: strings.ToLower(ro.Resource)}
			clusterResource, err := dynamicClient.Resource(gvr).Get(ctx, ro.Name, metav1.GetOptions{})
			if err != nil {
				klog.V(2).Infof("Unable to list %s resource due to: %s", gvr, err)
			}
			if clusterResource == nil {
				continue
			}
			kind, err := utils.NestedStringWrapper(clusterResource.Object, "kind")
			if err != nil {
				continue
			}
			apiVersion, err := utils.NestedStringWrapper(clusterResource.Object, "apiVersion")
			if err != nil {
				continue
			}
			name, err := utils.NestedStringWrapper(clusterResource.Object, "metadata", "name")
			if err != nil {
				continue
			}
			spec, ok := clusterResource.Object["spec"]
			if !ok {
				klog.Warningf("Can't find spec for cluster operator resource %s", name)
			}
			res = append(res, clusterOperatorResource{Spec: spec, Kind: kind, Name: name, APIVersion: apiVersion})
		}
	}
	return res
}

// getOperatorResourcesVersions get all the operator resource versions
func getOperatorResourcesVersions(discoveryClient discovery.DiscoveryInterface) (map[string][]string, error) {
	resources, err := discoveryClient.ServerPreferredResources()
	if err != nil {
		return nil, err
	}
	resourceVersionMap := make(map[string][]string)
	for _, v := range resources {
		if strings.Contains(v.GroupVersion, "operator.openshift.io") {
			gv, err := schema.ParseGroupVersion(v.GroupVersion)
			if err != nil {
				continue
			}
			for i := range v.APIResources {
				key := fmt.Sprintf("%s-%s", gv.Group, v.APIResources[i].Name)
				_, ok := resourceVersionMap[key]
				if !ok {
					resourceVersionMap[key] = []string{gv.Version}
					continue
				}
				resourceVersionMap[key] = append(resourceVersionMap[key], gv.Version)
			}
		}
	}
	return resourceVersionMap, nil
}
