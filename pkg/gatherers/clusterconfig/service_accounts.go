package clusterconfig

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
)

// GatherServiceAccounts Collects `ServiceAccount` stats
// from kubernetes default and `openshift-*` namespaces.
//
// ### API Reference
// - https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/serviceaccount.go#L83
// - https://docs.openshift.com/container-platform/4.3/rest_api/index.html#serviceaccount-v1-core
//
// ### Sample data
// - docs/insights-archive-sample/config/serviceaccounts.json
//
// ### Location in archive
// - `config/serviceaccounts.json`
//
// ### Config ID
// `clusterconfig/service_accounts`
//
// ### Released version
// - 4.7.0
//
// ### Backported versions
// - 4.5.34+
// - 4.6.20+
//
// ### Changes
// None
func (g *Gatherer) GatherServiceAccounts(ctx context.Context) ([]record.Record, []error) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherServiceAccounts(ctx, gatherKubeClient.CoreV1())
}

func gatherServiceAccounts(ctx context.Context, coreClient corev1client.CoreV1Interface) ([]record.Record, []error) {
	config, err := utils.GetAllNamespaces(ctx, coreClient)
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	totalServiceAccounts := 0
	var serviceAccounts []corev1.ServiceAccount
	var records []record.Record
	namespaces := defaultNamespaces
	// collect from all openshift-* namespaces + kubernetes defaults
	for i := range config.Items {
		if strings.HasPrefix(config.Items[i].Name, "openshift-") {
			namespaces = append(namespaces, config.Items[i].Name)
		}
	}
	// Maximal total number of service accounts
	var maxServiceAccountsLimit = 1000
	for _, namespace := range namespaces {
		// fetching service accounts from namespace
		svca, err := coreClient.ServiceAccounts(namespace).List(ctx, metav1.ListOptions{Limit: int64(maxServiceAccountsLimit)})
		if err != nil {
			klog.V(2).Infof("Unable to read ServiceAccounts in namespace %s error %s", namespace, err)
			continue
		}

		totalServiceAccounts += len(svca.Items)
		for j := range svca.Items {
			if len(serviceAccounts) > maxServiceAccountsLimit {
				break
			}
			serviceAccounts = append(serviceAccounts, svca.Items[j])
		}
	}

	records = append(records, record.Record{
		Name: "config/serviceaccounts",
		Item: ServiceAccountsMarshaller{serviceAccounts, totalServiceAccounts},
	})
	return records, nil
}

// ServiceAccountsMarshaller implements serialization of Service Accounts
type ServiceAccountsMarshaller struct {
	serviceAccounts      []corev1.ServiceAccount
	totalServiceAccounts int
}

type serviceAccountInfo struct {
	Name            string `json:"name"`
	NumberOfSecrets int    `json:"secrets"`
}

// Marshal implements serialization of ServiceAccount
func (a ServiceAccountsMarshaller) Marshal() ([]byte, error) {
	// Creates map for marshal
	serviceAccounts := map[string]any{}
	namespaces := map[string][]serviceAccountInfo{}
	serviceAccounts["serviceAccounts"] = map[string]any{
		"TOTAL_COUNT": a.totalServiceAccounts,
		"namespaces":  namespaces,
	}

	for i := range a.serviceAccounts {
		saInfo := serviceAccountInfo{
			Name:            a.serviceAccounts[i].Name,
			NumberOfSecrets: len(a.serviceAccounts[i].Secrets),
		}
		namespace := a.serviceAccounts[i].Namespace
		namespaces[namespace] = append(namespaces[namespace], saInfo)
	}
	return json.Marshal(serviceAccounts)
}

// GetExtension returns extension for anonymized openshift objects
func (a ServiceAccountsMarshaller) GetExtension() string {
	return record.JSONExtension
}
