package clusterconfig

import (
	"context"
	"fmt"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
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
	namespace  string
}

// GatherClusterOperators Collects all the ClusterOperators definitions and their related resources
// from the `operator.openshift.io` group.
//
// ### API Reference
// - https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/clusteroperator.go#L62
// - https://docs.openshift.com/container-platform/4.3/rest_api/index.html#clusteroperatorlist-v1config-openshift-io
//
// ### Sample data
// - docs/insights-archive-sample/config/clusteroperator
//
// ### Location in archive
// | Version   | Path														|
// | --------- | --------------------------------------------------------	|
// | >= 4.3    | config/clusteroperator/ 									|
// | < 4.7	   | config/clusteroperator/{kind}-{name}.json 					|
// | >= 4.7    | config/clusteroperator/{group}/{kind}/{name}.json 			|
//
// ### Config ID
// `clusterconfig/`
//
// ### Released version
// - 4.2.0
//
// ### Backported versions
// None
//
// ### Notes
// The `ClusterOperators` were used to also collect `pods` and `events`, it changed at the `4.8.2` release. The `pods`
// and `events` gathering were introduced in `4.3` and backported to `4.2.10`.
//
// * Spec config for CO resources was introduced at `4.7.0` and backported to`4.6.16+`
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
	resVer, err := getOperatorResourcesVersions(discoveryClient)
	if err != nil {
		klog.Warning("Can't read operator resource versions: %v", err)
	}
	records := make([]record.Record, 0, len(items))

	for idx := range items {
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/clusteroperator/%s", items[idx].Name),
			Item: record.ResourceMarshaller{Resource: &items[idx]},
		})
		if resVer == nil {
			continue
		}

		relRes := collectClusterOperatorRelatedObjects(ctx, dynamicClient, items[idx], resVer)
		for _, rr := range relRes {
			// imageregistry resources (config, pruner) are gathered in image_registries.go, image_pruners.go
			if strings.Contains(rr.APIVersion, "imageregistry") {
				continue
			}
			gv, err := schema.ParseGroupVersion(rr.APIVersion)
			if err != nil {
				klog.Warningf("Unable to parse group version %s: %s", rr.APIVersion, err)
			}
			recName := fmt.Sprintf("config/clusteroperator/%s/%s/%s", gv.Group, strings.ToLower(rr.Kind), rr.Name)
			if rr.namespace != "" {
				recName = fmt.Sprintf("config/clusteroperator/%s/%s/%s/%s", gv.Group, strings.ToLower(rr.Kind), rr.namespace, rr.Name)
			}
			records = append(records, record.Record{
				Name: recName,
				Item: record.JSONMarshaller{Object: rr},
			})
		}
	}

	return records
}

// collectClusterOperatorRelatedObjects iterates over all the clusteroperator relatedObjects
// and stores all the objects from "operator.openshift.io" group. Then it tries to read all
// found resources in this group and store it in the archive.
func collectClusterOperatorRelatedObjects(ctx context.Context,
	dynamicClient dynamic.Interface,
	co configv1.ClusterOperator, //nolint: gocritic
	resVer map[schema.GroupResource]string) []clusterOperatorResource {
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
		gr := schema.GroupResource{
			Group:    ro.Group,
			Resource: ro.Resource,
		}
		version := resVer[gr]
		gvr := gr.WithVersion(version)
		clusterResource, err := getRelatedObjectResource(ctx, dynamicClient, ro, co.Name, gvr)
		if err != nil {
			klog.V(2).Infof("Unable to get %s resource due to: %s", fmt.Sprintf("%s.%s", ro.Resource, ro.Group), err)
			continue
		}
		spec, ok := clusterResource.Object["spec"]
		if !ok {
			klog.Warningf("Can't find spec for cluster operator resource %s", clusterResource.GetName())
		}
		anonymizeIdentityProviders(clusterResource.Object)
		res = append(res, clusterOperatorResource{
			Spec:       spec,
			Kind:       clusterResource.GetKind(),
			Name:       clusterResource.GetName(),
			APIVersion: clusterResource.GetAPIVersion(),
			namespace:  clusterResource.GetNamespace(),
		})
	}
	return res
}

// getRelatedObjectResource gets/reads the related object (based on the attribtues passed in the ObjectReference)
// from the cluster API using the dynamic Interface. It handles some extra cases, when the relatedObject name is not availab.e
func getRelatedObjectResource(ctx context.Context,
	dynamicClient dynamic.Interface,
	ro configv1.ObjectReference,
	coName string, gvr schema.GroupVersionResource) (*unstructured.Unstructured, error) {
	// ingress cluster operator has related object ingresscontroller, but the name is not provided
	// there is the default one with name "default"
	if ro.Name == "" && coName == "ingress" {
		ro.Name = "default"
	}
	return dynamicClient.Resource(gvr).Namespace(ro.Namespace).Get(ctx, ro.Name, metav1.GetOptions{})
}

// getOperatorResourcesVersions get all the operator resource versions
func getOperatorResourcesVersions(discoveryClient discovery.DiscoveryInterface) (map[schema.GroupResource]string, error) {
	resources, err := discoveryClient.ServerPreferredResources()
	if err != nil {
		return nil, err
	}
	resourceVersionMap := make(map[schema.GroupResource]string)
	for _, v := range resources {
		if strings.Contains(v.GroupVersion, "operator.openshift.io") {
			gv, err := schema.ParseGroupVersion(v.GroupVersion)
			if err != nil {
				continue
			}
			for i := range v.APIResources {
				gr := schema.GroupResource{
					Group:    gv.Group,
					Resource: v.APIResources[i].Name,
				}
				resourceVersionMap[gr] = gv.Version
			}
		}
	}
	return resourceVersionMap, nil
}

// anonymizeIdentityProviders tries to get an array of identity providers defined in OAuth config
// and anonymize potentially sensitive data - e.g LDAP domain, url
func anonymizeIdentityProviders(obj map[string]interface{}) {
	ips, err := utils.NestedSliceWrapper(obj, "spec", "observedConfig", "oauthServer", "oauthConfig", "identityProviders")

	// most of the clusteroperator resources will not have any identity provider config so silence the error
	if err != nil {
		return
	}
	sensittiveProviderAttributes := []string{"url", "bindDN", "hostname", "clientID", "hostedDomain", "issuer", "domainName"}
	for _, ip := range ips {
		ip, ok := ip.(map[string]interface{})
		if !ok {
			klog.Warningln("Failed to convert %v to map[string]interface{}", ip)
			continue
		}
		for _, sensitiveVal := range sensittiveProviderAttributes {
			// check if the sensitive value is in the provider definition under "provider" attribute
			// and overwrite only if exists
			if val, err := utils.NestedStringWrapper(ip, "provider", sensitiveVal); err == nil {
				_ = unstructured.SetNestedField(ip, anonymize.String(val), "provider", sensitiveVal)
			}
		}
	}
	_ = unstructured.SetNestedSlice(obj, ips, "spec", "observedConfig", "oauthServer", "oauthConfig", "identityProviders")
}
