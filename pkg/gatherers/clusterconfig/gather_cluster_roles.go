package clusterconfig

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/record"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/rbac/v1"
)

// GatherClusterRoles Collects definition of the "admin" and "edit" cluster roles.
//
// ### API Reference
// - https://github.com/kubernetes/kubernetes/blob/master/pkg/apis/rbac/types.go
//
// ### Sample data
// - docs/insights-archive-sample/cluster-scoped-resources/rbac.authorization.k8s.io/clusterroles
//
// ### Location in archive
// - `cluster-scoped-resources/rbac.authorization.k8s.io/clusterroles/`
//
// ### Config ID
// `clusterconfig/clusterroles`
//
// ### Released version
// - 4.18.0
//
// ### Backported versions
//
// ### Changes
// None
func (g *Gatherer) GatherClusterRoles(ctx context.Context) ([]record.Record, []error) {
	kubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherClusterRoles(ctx, kubeClient.RbacV1(), []string{"admin", "edit"})
}

func gatherClusterRoles(ctx context.Context, rbacV1Cli v1.RbacV1Interface, names []string) ([]record.Record, []error) {
	var errs []error
	var records []record.Record
	for _, name := range names {
		clusterRoleRec, err := gatherClusterRole(ctx, name, rbacV1Cli)
		if err != nil {
			errs = append(errs, err)
		} else {
			records = append(records, *clusterRoleRec)
		}
	}
	return records, errs
}

func gatherClusterRole(ctx context.Context, name string, rbacV1Cli v1.RbacV1Interface) (*record.Record, error) {
	clusterRole, err := rbacV1Cli.ClusterRoles().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return &record.Record{
		Name: fmt.Sprintf("cluster-scoped-resources/rbac.authorization.k8s.io/clusterroles/%s", clusterRole.Name),
		Item: record.ResourceMarshaller{Resource: clusterRole},
	}, nil
}
