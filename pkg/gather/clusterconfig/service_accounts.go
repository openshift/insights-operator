package clusterconfig

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog"

	_ "k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	"github.com/openshift/insights-operator/pkg/record"
)

// Maximal total number of service accounts
const maxServiceAccountsLimit = 1000

// GatherServiceAccounts collects ServiceAccount stats
// from kubernetes default and namespaces starting with openshift.
//
// The Kubernetes api https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/serviceaccount.go#L83
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#serviceaccount-v1-core
//
// Location of serviceaccounts in archive: config/serviceaccounts
// See: docs/insights-archive-sample/config/serviceaccounts
func GatherServiceAccounts(g *Gatherer) ([]record.Record, []error) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}
	return gatherServiceAccounts(g.ctx, gatherKubeClient.CoreV1())
}

func gatherServiceAccounts(ctx context.Context, coreClient corev1client.CoreV1Interface) ([]record.Record, []error) {
	config, ctx, err := getAllNamespaces(ctx, coreClient)
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	totalServiceAccounts := 0
	serviceAccounts := []corev1.ServiceAccount{}
	records := []record.Record{}
	namespaces := defaultNamespaces
	// collect from all openshift* namespaces + kubernetes defaults
	for _, item := range config.Items {
		if strings.HasPrefix(item.Name, "openshift") {
			namespaces = append(namespaces, item.Name)
		}
	}
	for _, namespace := range namespaces {
		// fetching service accounts from namespace
		svca, err := coreClient.ServiceAccounts(namespace).List(ctx, metav1.ListOptions{Limit: maxServiceAccountsLimit})
		if err != nil {
			klog.V(2).Infof("Unable to read ServiceAccounts in namespace %s error %s", namespace, err)
			continue
		}

		totalServiceAccounts += len(svca.Items)
		for _, j := range svca.Items {
			if len(serviceAccounts) > maxServiceAccountsLimit {
				break
			}
			serviceAccounts = append(serviceAccounts, j)
		}
	}

	records = append(records, record.Record{Name: fmt.Sprintf("config/serviceaccounts"), Item: ServiceAccountsMarshaller{serviceAccounts, totalServiceAccounts}})
	return records, nil
}

// ServiceAccountsMarshaller implements serialization of Service Accounts
type ServiceAccountsMarshaller struct {
	sa                   []corev1.ServiceAccount
	totalServiceAccounts int
}

// Marshal implements serialization of ServiceAccount
func (a ServiceAccountsMarshaller) Marshal(_ context.Context) ([]byte, error) {
	// Creates map for marshal
	sr := map[string]interface{}{}
	st := map[string]interface{}{}
	st["TOTAL_COUNT"] = a.totalServiceAccounts
	sr["serviceAccounts"] = st
	nss := map[string]interface{}{}
	st["namespaces"] = nss
	for _, sa := range a.sa {
		var ns map[string]interface{}
		var ok bool
		if _, ok = nss[sa.Namespace]; !ok {
			ns = map[string]interface{}{}
			nss[sa.Namespace] = ns
		} else {
			ns = nss[sa.Namespace].(map[string]interface{})
		}
		ns["name"] = sa.Name
		ns["secrets"] = len(sa.Secrets)
	}
	return json.Marshal(sr)
}

// GetExtension returns extension for anonymized openshift objects
func (a ServiceAccountsMarshaller) GetExtension() string {
	return "json"
}
