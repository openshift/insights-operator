package clusterconfig

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	batchv1client "k8s.io/client-go/kubernetes/typed/batch/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherSAPPods collects information about pods running in SAP/SDI namespaces.
// Only pods with a failing status are collected.
// Failed pods belonging to a job that has later succeeded are ignored.
//
// Relevant Kubernetes API docs:
//   - https://pkg.go.dev/k8s.io/client-go/kubernetes/typed/core/v1
//   - https://pkg.go.dev/k8s.io/client-go/kubernetes/typed/batch/v1
//   - https://pkg.go.dev/k8s.io/client-go/dynamic
//
// * Location in archive: config/pod/{namespace}/{pod-name}.json
// * Since versions:
//   * 4.8+
func GatherSAPPods(g *Gatherer, c chan<- gatherResult) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{errors: []error{err}}
		return
	}
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		c <- gatherResult{errors: []error{err}}
		return
	}
	gatherJobsClient, err := batchv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{errors: []error{err}}
		return
	}

	records, errors := gatherSAPPods(g.ctx, gatherDynamicClient, gatherKubeClient.CoreV1(), gatherJobsClient)
	c <- gatherResult{records: records, errors: errors}
}

func gatherSAPPods(ctx context.Context, dynamicClient dynamic.Interface, coreClient corev1client.CoreV1Interface, jobsClient batchv1client.BatchV1Interface) ([]record.Record, []error) {
	datahubsResource := schema.GroupVersionResource{Group: "installers.datahub.sap.com", Version: "v1alpha1", Resource: "datahubs"}

	datahubsList, err := dynamicClient.Resource(datahubsResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	records := []record.Record{}
	collectedNamespaces := map[string]struct{}{}
	for _, datahub := range datahubsList.Items {
		datahubNamespace := datahub.GetNamespace()
		if _, exists := collectedNamespaces[datahubNamespace]; exists {
			continue
		}
		collectedNamespaces[datahubNamespace] = struct{}{}

		pods, err := coreClient.Pods(datahubNamespace).List(ctx, metav1.ListOptions{})
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, []error{err}
		}

		for iPod, pod := range pods.Items {
			// Skip pods that are running correctly or those that have already successfully finished.
			if pod.Status.Phase == v1.PodRunning || pod.Status.Phase == v1.PodSucceeded {
				continue
			}

			// Indicates if the pod belongs to a successful job.
			successfulJob := false
			for _, owner := range pod.ObjectMeta.OwnerReferences {
				if owner.Kind != "Job" {
					continue
				}

				ownerJob, err := jobsClient.Jobs(pod.Namespace).Get(ctx, owner.Name, metav1.GetOptions{})
				if err != nil {
					return nil, []error{err}
				}

				if ownerJob.Status.Succeeded > 0 {
					successfulJob = true
					break
				}
			}
			// If the job succeeded using a different pod after this pod failed,
			// this pod is no longer relevant and should not be gathered.
			if successfulJob {
				continue
			}

			records = append(records, record.Record{
				// There are already some (OpenShift/OCP) pods in `/config/pod/**`
				Name: fmt.Sprintf("config/pod/%s/%s", pod.Namespace, pod.Name),
				// It is impossible to use `&pod` here because it would end up being
				// the last returned pod as the reference keeps changing with each iteration.
				Item: record.JSONMarshaller{Object: &pods.Items[iPod]},
			})
		}
	}

	return records, nil
}
