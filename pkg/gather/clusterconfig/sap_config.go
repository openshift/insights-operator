package clusterconfig

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"github.com/openshift/insights-operator/pkg/record"
)

// GatherSAPConfig collects selected security context constraints
// and cluster role bindings from clusters running a SAP payload.
//
// Relevant OpenShift API docs:
//   - https://pkg.go.dev/github.com/openshift/client-go/authorization/clientset/versioned/typed/authorization/v1
//   - https://pkg.go.dev/github.com/openshift/client-go/security/clientset/versioned/typed/security/v1
//
// Location in archive: config/securitycontentconstraint/, config/clusterrolebinding/
func GatherSAPConfig(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		sccToGather := []string{"anyuid", "privileged"}
		crbToGather := []string{"system:openshift:scc:anyuid", "system:openshift:scc:privileged"}

		datahubsResource := schema.GroupVersionResource{Group: "installers.datahub.sap.com", Version: "v1alpha1", Resource: "datahubs"}

		datahubsList, err := i.dynamicClient.Resource(datahubsResource).List(i.ctx, metav1.ListOptions{})

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
			scc, err := i.securityClient.SecurityContextConstraints().Get(i.ctx, name, metav1.GetOptions{})
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
			crb, err := i.authClient.ClusterRoleBindings().Get(i.ctx, name, metav1.GetOptions{})
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
}
