package clusterconfig

import (
	"context"
	"fmt"
	"strings"

	authclient "github.com/openshift/client-go/authorization/clientset/versioned/typed/authorization/v1"
	securityv1client "github.com/openshift/client-go/security/clientset/versioned/typed/security/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherSAPConfig collects selected security context constraints
// and cluster role bindings from clusters running a SAP payload.
//
// **Conditional data**: This data is collected only if the "installers.datahub.sap.com" resource is found in the cluster.
//
// Relevant OpenShift API docs:
//   - https://pkg.go.dev/github.com/openshift/client-go/authorization/clientset/versioned/typed/authorization/v1
//   - https://pkg.go.dev/github.com/openshift/client-go/security/clientset/versioned/typed/security/v1
//
// * Location in archive: config/securitycontentconstraint/, config/clusterrolebinding/
// * Since versions:
//   * 4.6.20+
//   * 4.7+
func (g *Gatherer) GatherSAPConfig(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	gatherSecurityClient, err := securityv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	gatherAuthClient, err := authclient.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherSAPConfig(ctx, gatherDynamicClient, gatherSecurityClient, gatherAuthClient)
}

func gatherSAPConfig(ctx context.Context,
	dynamicClient dynamic.Interface,
	securityClient securityv1client.SecurityV1Interface,
	authClient authclient.AuthorizationV1Interface) ([]record.Record, []error) {
	sccToGather := []string{"anyuid", "privileged"}
	crbToGather := []string{"system:openshift:scc:anyuid", "system:openshift:scc:privileged"}

	datahubsList, err := dynamicClient.Resource(datahubGroupVersionResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	// If no DataHubs resource exists on the cluster, skip this gathering.
	// This may already be handled by the IsNotFound check, but it's better to be sure.
	if len(datahubsList.Items) == 0 {
		return nil, nil
	}

	var records []record.Record

	for _, name := range sccToGather {
		scc, err := securityClient.SecurityContextConstraints().Get(ctx, name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, []error{err}
		}
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/securitycontextconstraint/%s", scc.Name),
			// It is not possible to use the generic OpenShift Anonymizer type here
			// because the SCC and CRB resources returned by their respective clients
			// are currently missing some properties (kind, apiVersion).
			Item: record.ResourceMarshaller{Resource: scc},
		})
	}

	for _, name := range crbToGather {
		crb, err := authClient.ClusterRoleBindings().Get(ctx, name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, []error{err}
		}
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/clusterrolebinding/%s", strings.ReplaceAll(crb.Name, ":", "_")),
			// See the note above on why it is not possible to use the generic Anonymizer type.
			Item: record.ResourceMarshaller{Resource: crb},
		})
	}

	return records, nil
}
