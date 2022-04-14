package clusterconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

type uidRange struct {
	starting int64
	length   int64
}

type namespaceWithRange struct {
	name string
	uidRange
}

// IsOverlappingWith checks if the UIDRange is overlapping with the provided one
func (u uidRange) IsOverlappingWith(r uidRange) bool {
	uSum := u.starting + u.length
	rSum := r.starting + r.length
	return (uSum > r.starting && uSum <= rSum) || (rSum > u.starting && rSum <= uSum)
}

func (u uidRange) String() string {
	return fmt.Sprintf("%d/%d", u.starting, u.length)
}

// GatherNamespacesWithOverlappingUIDs gathers namespaces with overlapping UID ranges
//
// The Kubernetes api https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/namespace.go
// Response is an array of arrays of namespaces with overlapping UIDs. Each namespace is represented by its name and the UID range value
// from the "openshift.io/sa.scc.uid-range" annotation
//
// * Location in archive: config/namespaces_with_overlapping_uids
// * Id in config: clusterconfig/overlapping_namespace_uids
// * Since versions:
//   * 4.11+
func (g *Gatherer) GatherNamespacesWithOverlappingUIDs(ctx context.Context) ([]record.Record, []error) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}
	return gatherNamespacesWithOverlappingUIDs(ctx, gatherKubeClient.CoreV1())
}

func gatherNamespacesWithOverlappingUIDs(ctx context.Context, coreClient corev1client.CoreV1Interface) ([]record.Record, []error) {
	nsList, err := coreClient.Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, []error{err}
	}
	var namespaces []namespaceWithRange
	var errs []error
	for i := range nsList.Items {
		ns := nsList.Items[i]
		uidRangeString := ns.Annotations["openshift.io/sa.scc.uid-range"]
		r, err := uidStringToRange(uidRangeString)
		if err != nil {
			errs = append(errs, fmt.Errorf("can't read uid range of the %s namespace", ns.Name))
			continue
		}
		namespaces = append(namespaces, namespaceWithRange{ns.Name, *r})
	}
	var resultSet SetOfNamespaceSets
	for i := range namespaces {
		n1 := namespaces[i]
		// remove first i+1 elements from the slice so that we don't iterate over them again
		remainingNs := namespaces[i+1:]
		for j := range remainingNs {
			n2 := remainingNs[j]
			if n1.IsOverlappingWith(n2.uidRange) {
				if es, ok := resultSet.BothOverlap(n1, n2); ok {
					es.Insert(n1, n2)
				} else {
					s := NewSet()
					s.Insert(n1, n2)
					resultSet = append(resultSet, s)
				}
			}
		}
	}
	return []record.Record{{
		Name: "config/namespaces_with_overlapping_uids",
		Item: resultSet,
	}}, errs
}

// uidStringToRange converts string UID range to `UIDRange` type
func uidStringToRange(s string) (*uidRange, error) {
	values := strings.Split(s, "/")
	starting, err := strconv.Atoi(values[0])
	if err != nil {
		return nil, err
	}
	rge, err := strconv.Atoi(values[1])
	if err != nil {
		return nil, err
	}
	return &uidRange{
		starting: int64(starting),
		length:   int64(rge),
	}, nil
}

type NamespaceSet map[namespaceWithRange]struct{}

// NewSet creates a set of namesapces from a list of values.
func NewSet(namespaces ...namespaceWithRange) NamespaceSet {
	ns := NamespaceSet{}
	ns.Insert(namespaces...)
	return ns
}

// Insert adds namespaces to the set.
func (ns NamespaceSet) Insert(namespaces ...namespaceWithRange) NamespaceSet {
	for _, n := range namespaces {
		ns[n] = struct{}{}
	}
	return ns
}

// BothOverlap checks if the namespaces n1 and n2 are overlapping
// with all ranges in the set
func (ns NamespaceSet) BothOverlap(n1, n2 namespaceWithRange) bool {
	if len(ns) == 0 {
		return false
	}
	for k := range ns {
		if !n1.IsOverlappingWith(k.uidRange) || !n2.IsOverlappingWith(k.uidRange) {
			return false
		}
	}
	return true
}

type SetOfNamespaceSets []NamespaceSet

// BothOverlap tries to find a NamespaceSet where all the members overlap with n1 and n2
func (ss SetOfNamespaceSets) BothOverlap(n1, n2 namespaceWithRange) (NamespaceSet, bool) {
	for _, set := range ss {
		if set.BothOverlap(n1, n2) {
			return set, true
		}
	}
	return nil, false
}

func (ss SetOfNamespaceSets) Marshal() ([]byte, error) {
	result := make([][]string, 0, len(ss))
	for _, set := range ss {
		var overlapping []string
		for s := range set {
			overlapping = append(overlapping, s.name)
		}
		result = append(result, overlapping)
	}
	return json.Marshal(result)
}

func (ss SetOfNamespaceSets) GetExtension() string {
	return "json"
}
