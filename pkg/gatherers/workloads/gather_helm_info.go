package workloads

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

const labelChartNameKey = "helm.sh/chart"

// GatherHelmInfo Collects statistics about resources deployed via HelmChart, counting only the resources
// with `app.kubernetes.io/managed-by=Helm` and `helm.sh/chart` labels. The data is then summarized
// and grouped by hashed namespace.
//
// Resource types included:
// - ReplicaSets
// - DaemonSets
// - StatefulSets
// - Services
// - Deployments
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/config/helmchart_info.json
//
// ### Location in archive
// - `config/helmchart_info.json`
//
// ### Config ID
// `workloads/helmchart_info`
//
// ### Released version
// - 4.15.0
//
// ### Backported versions
// None
func (g *Gatherer) GatherHelmInfo(ctx context.Context) ([]record.Record, []error) {
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherHelmInfo(ctx, dynamicClient)
}

func gatherHelmInfo(
	ctx context.Context,
	dynamicClient dynamic.Interface,
) ([]record.Record, []error) {
	resources := []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "replicasets"},
		{Group: "apps", Version: "v1", Resource: "daemonsets"},
		{Group: "apps", Version: "v1", Resource: "statefulsets"},
		{Group: "", Version: "v1", Resource: "services"},
		{Group: "apps", Version: "v1", Resource: "deployments"},
	}

	var errs []error
	var records []record.Record
	helmList := newHelmChartInfoList()

	for _, resource := range resources {
		listOptions := metav1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=Helm"}

		items, err := dynamicClient.Resource(resource).List(ctx, listOptions)
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			klog.V(2).Infof("Unable to list %s resource due to: %s", resource, err)
			errs = append(errs, err)
			continue
		}

		for _, item := range items.Items {
			labels := item.GetLabels()

			// Anonymize the namespace to make it unique identifier
			hash, err := createHash(item.GetNamespace())
			if err != nil {
				klog.Errorf("unable to hash the namespace '%s': %v", item.GetNamespace(), err)
				continue
			}

			name, version := helmChartNameAndVersion(labels[labelChartNameKey])
			if name == "" && version == "" {
				// some helm-maneged resource may not have reference to the chart
				klog.Infof("unable to get HelmChart from %s on %s from %s.", resource.Resource, item.GetNamespace(), item.GetName())
				continue
			}

			helmList.addItem(hash, resource.Resource, HelmChartInfo{
				Name:    name,
				Version: version,
			})
		}
	}

	if len(helmList.Namespaces) > 0 {
		records = []record.Record{
			{
				Name: "config/helmchart_info",
				Item: record.JSONMarshaller{Object: &helmList.Namespaces},
			},
		}
	}

	if len(errs) > 0 {
		return records, errs
	}

	return records, nil
}

func createHash(chartName string) (string, error) {
	h := sha256.New()
	_, err := h.Write([]byte(chartName))
	if err != nil {
		return "", err
	}

	hashInBytes := h.Sum(nil)
	hash := hex.EncodeToString(hashInBytes)

	return hash, nil
}

func helmChartNameAndVersion(chart string) (name, version string) {
	parts := strings.Split(chart, "-")

	// no version found
	if len(parts) == 1 {
		return chart, ""
	}

	name = strings.Join(parts[:len(parts)-1], "-")

	// best guess to get the version
	version = parts[len(parts)-1]
	// check for standard version format
	if !strings.Contains(version, ".") {
		// maybe it is a string version
		if !isStringVersion(version) {
			// not a valid version, add to name and version should be empty
			name = fmt.Sprintf("%s-%s", name, version)
			version = ""
		}
	}

	return name, version
}

func isStringVersion(version string) bool {
	stringVersions := []string{"latest", "beta", "alpha"}
	for _, v := range stringVersions {
		if v == version {
			return true
		}
	}
	return false
}
