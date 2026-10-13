package clusterconfig

import (
	"context"
	"regexp"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
)

// Regex used to remove a trailing hyphen and number suffix (e.g., "-123" at the end of a string)
var revisionSuffixRegex = regexp.MustCompile(`-\d+$`)

// GatherRevisionedObjectCounts collects revision counts for ConfigMap and Secret
// objects with revision-based naming in specific namespaces.
//
// It groups objects by base name (removing the -<number> suffix) and counts the
// number of revisions per base name. This helps identify objects with excessive
// historical revisions (>20 or >50) that may impact cluster performance and should
// be cleaned up via a pruner.
//
// Example output for openshift-kube-apiserver namespace:
//   - encryption-config: 590 revisions
//   - etcd-client: 608 revisions
//   - config: 609 revisions
//
// The namespaces to monitor are defined in revisionedObjectNamespaces
// and can be extended by modifying the const.go file.
//
// ### API Reference
// - https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/configmap.go
// - https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/secret.go
//
// ### Sample data
// - docs/insights-archive-sample/config/versioned_object_revision_counts.json
//
// ### Location in archive
// - `config/versioned_object_revision_counts.json`
//
// ### Config ID
// `clusterconfig/revisioned_objects`
//
// ### Released version
// - TBD
//
// ### Changes
// None
func (g *Gatherer) GatherRevisionedObjectCounts(ctx context.Context) ([]record.Record, []error) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherRevisionedObjectCounts(ctx, gatherKubeClient.CoreV1())
}

func gatherRevisionedObjectCounts(ctx context.Context, coreClient corev1client.CoreV1Interface) ([]record.Record, []error) {
	namespaceCounts := make(map[string]*NamespaceRevisionCounts)

	for _, namespace := range revisionedObjectNamespaces {
		nsCounts := &NamespaceRevisionCounts{
			ConfigMaps: make(map[string]int),
			Secrets:    make(map[string]int),
		}

		// Gather ConfigMap counts
		configMaps, err := coreClient.ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			klog.V(2).Infof("Unable to read ConfigMaps in namespace %s: %v", namespace, err)
		} else {
			for i := range configMaps.Items {
				cm := &configMaps.Items[i]
				if hasRevisionStatusOwner(cm.OwnerReferences) {
					baseName := extractBaseName(cm.Name)
					nsCounts.ConfigMaps[baseName]++
				}
			}
		}

		// Gather Secret counts
		secrets, err := coreClient.Secrets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			klog.V(2).Infof("Unable to read Secrets in namespace %s: %v", namespace, err)
		} else {
			for i := range secrets.Items {
				secret := &secrets.Items[i]
				if hasRevisionStatusOwner(secret.OwnerReferences) {
					baseName := extractBaseName(secret.Name)
					nsCounts.Secrets[baseName]++
				}
			}
		}

		// Only add namespace to output if it has any revisioned objects
		if len(nsCounts.ConfigMaps) > 0 || len(nsCounts.Secrets) > 0 {
			namespaceCounts[namespace] = nsCounts
		}
	}

	// Return single record with all counts
	return []record.Record{{
		Name: "config/versioned_object_revision_counts",
		Item: record.JSONMarshaller{Object: namespaceCounts},
	}}, nil
}

// extractBaseName removes the revision suffix (-123) from object names
// e.g., "encryption-config-590" -> "encryption-config"
func extractBaseName(name string) string {
	return revisionSuffixRegex.ReplaceAllString(name, "")
}

// hasRevisionStatusOwner checks if object has ownerReference starting with "revision-status-"
func hasRevisionStatusOwner(ownerRefs []metav1.OwnerReference) bool {
	if len(ownerRefs) == 0 {
		return false
	}
	for _, ref := range ownerRefs {
		if strings.HasPrefix(ref.Name, "revision-status-") {
			return true
		}
	}
	return false
}

// NamespaceRevisionCounts contains revision counts for ConfigMaps and Secrets
type NamespaceRevisionCounts struct {
	ConfigMaps map[string]int `json:"configmaps"`
	Secrets    map[string]int `json:"secrets"`
}
