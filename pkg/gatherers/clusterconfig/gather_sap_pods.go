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

// GatherSAPPods Collects information about pods running in SAP/SDI namespaces.
//
// - Only pods with a failing status are collected.
// - Failed pods belonging to a job that has later succeeded are ignored.
//
// > **Note**
// > This data is collected only if the `installers.datahub.sap.com` resource is found in the cluster.
//
// ### API Reference
// - https://pkg.go.dev/k8s.io/client-go/kubernetes/typed/core/v1
// - https://pkg.go.dev/k8s.io/client-go/kubernetes/typed/batch/v1
// - https://pkg.go.dev/k8s.io/client-go/dynamic
//
// ### Sample data
// - docs/insights-archive-sample/config/pod/di-288312/auditlog-retention-28566720-t22qj.json
// - docs/insights-archive-sample/config/pod/di-288312/data-hub-flow-agent-1a3a7e88888b7fe0630189-qcwhm-547b57cc5fvmg8.json
// - docs/insights-archive-sample/config/pod/di-288312/default-2k58azz-backup-deletion-5rdw4.json
// - docs/insights-archive-sample/config/pod/di-288312/vsystem-867f4b77cc-pqcns.json
//
// ### Location in archive
// - `config/pod/{namespace}/{name}.json`
//
// ### Config ID
// `clusterconfig/sap_pods`
//
// ### Released version
// - 4.8.2
//
// ### Backported versions
// - 4.7.5+
// - 4.6.25+
//
// ### Changes
// None
func (g *Gatherer) GatherSAPPods(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	gatherJobsClient, err := batchv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherSAPPods(ctx, gatherDynamicClient, gatherKubeClient.CoreV1(), gatherJobsClient)
}

func gatherSAPPods(ctx context.Context,
	dynamicClient dynamic.Interface,
	coreClient corev1client.CoreV1Interface,
	jobsClient batchv1client.BatchV1Interface) ([]record.Record, []error) {
	datahubsResource := schema.GroupVersionResource{Group: "installers.datahub.sap.com", Version: "v1alpha1", Resource: "datahubs"}

	datahubsList, err := dynamicClient.Resource(datahubsResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	var records []record.Record
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

		for i := range pods.Items {
			// Skip pods that are running correctly or those that have already successfully finished.
			if pods.Items[i].Status.Phase == v1.PodRunning || pods.Items[i].Status.Phase == v1.PodSucceeded {
				continue
			}

			// Indicates if the pod belongs to a successful job.
			successfulJob := false
			for _, owner := range pods.Items[i].ObjectMeta.OwnerReferences {
				if owner.Kind != "Job" {
					continue
				}

				ownerJob, err := jobsClient.Jobs(pods.Items[i].Namespace).Get(ctx, owner.Name, metav1.GetOptions{})
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
				Name: fmt.Sprintf("config/pod/%s/%s", pods.Items[i].Namespace, pods.Items[i].Name),
				// It is impossible to use `&pod` here because it would end up being
				// the last returned pod as the reference keeps changing with each iteration.
				Item: record.ResourceMarshaller{Resource: &pods.Items[i]},
			})
		}
	}

	return records, nil
}
