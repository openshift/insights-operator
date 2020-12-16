package clusterconfig

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	appsclient "k8s.io/client-go/kubernetes/typed/apps/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"

	appsv1 "k8s.io/api/apps/v1"
	_ "k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	"github.com/openshift/insights-operator/pkg/record"
)

//GatherStatefulSets collects StatefulSet configs from default namespaces
//
// The Kubernetes API https://github.com/kubernetes/api/blob/master/apps/v1/types.go
// Response see https://docs.openshift.com/container-platform/4.5/rest_api/workloads_apis/statefulset-apps-v1.html#statefulset-apps-v1
//
// Location in archive: config/statefulsets/
// Id in config: stateful_sets
func GatherStatefulSets(g *Gatherer) ([]record.Record, []error) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}
	appsClient, err := appsclient.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}
	return gatherStatefulSets(g.ctx, gatherKubeClient.CoreV1(), appsClient)
}

func gatherStatefulSets(ctx context.Context, coreClient corev1client.CoreV1Interface, appsClient appsclient.AppsV1Interface) ([]record.Record, []error) {
	namespaces, ctx, err := getAllNamespaces(ctx, coreClient)
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	osNamespaces := defaultNamespaces
	for _, item := range namespaces.Items {
		if strings.HasPrefix(item.Name, "openshift") {
			osNamespaces = append(osNamespaces, item.Name)
		}
	}
	records := []record.Record{}
	for _, namespace := range osNamespaces {
		sets, err := appsClient.StatefulSets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			klog.V(2).Infof("Unable to read StatefulSets in namespace %s error %s", namespace, err)
			continue
		}

		for i := range sets.Items {
			records = append(records, record.Record{
				Name: fmt.Sprintf("config/statefulsets/%s/%s", namespace, sets.Items[i].Name),
				Item: StatefulSetAnonymizer{&sets.Items[i]},
			})
		}
	}
	return records, nil
}

// StatefulSetAnonymizer implements StatefulSet serialization without anonymization
type StatefulSetAnonymizer struct{ *appsv1.StatefulSet }

// Marshal implements StatefulSet serialization
func (a StatefulSetAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(appsV1Serializer, a.StatefulSet)
}

// GetExtension returns extension for StatefulSet object
func (a StatefulSetAnonymizer) GetExtension() string {
	return "json"
}
