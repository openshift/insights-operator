package clusterconfig

import (
	"fmt"
	"context"
	"strings"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	authclient "github.com/openshift/client-go/authorization/clientset/versioned/typed/authorization/v1"
	securityv1client "github.com/openshift/client-go/security/clientset/versioned/typed/security/v1"
	"github.com/openshift/insights-operator/pkg/record"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GatherSAPConfig collects selected security context constraints
// and cluster role bindings from clusters running a SAP payload.
//
// Relevant OpenShift API docs:
//   - https://pkg.go.dev/github.com/openshift/client-go/authorization/clientset/versioned/typed/authorization/v1
//   - https://pkg.go.dev/github.com/openshift/client-go/security/clientset/versioned/typed/security/v1
//
// Location in archive: config/securitycontentconstraint/, config/clusterrolebinding/
func GatherSAPConfig(g *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
		if err != nil {
			return nil, []error{err}
		}
		gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
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
		return gatherSAPConfig(g.ctx, gatherDynamicClient, gatherKubeClient.CoreV1(), gatherSecurityClient, gatherAuthClient)
	}
}

func gatherSAPConfig(ctx context.Context, dynamicClient dynamic.Interface, coreClient corev1client.CoreV1Interface, securityClient securityv1client.SecurityV1Interface, authClient authclient.AuthorizationV1Interface) ([]record.Record, []error) {
	sccToGather := []string{"anyuid", "privileged"}
	crbToGather := []string{"system:openshift:scc:anyuid", "system:openshift:scc:privileged"}

	datahubsResource := schema.GroupVersionResource{Group: "installers.datahub.sap.com", Version: "v1alpha1", Resource: "datahubs"}

	datahubsList, err := dynamicClient.Resource(datahubsResource).List(ctx, metav1.ListOptions{})

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

	records := []record.Record{}

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
			Item: record.JSONMarshaller{Object: scc},
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
			Item: record.JSONMarshaller{Object: crb},
		})
	}

	return records, nil
}
