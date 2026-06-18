package conditional

import (
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
)

// BuildGatherPodDefinition Collects pod definitions from pods matching a specified prefix
// when the configured alert is firing.
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/conditional/namespaces/openshift-monitoring/pods/alertmanager-main-0/alertmanager-main-0.json
//
// ### Location in archive
// - `conditional/namespaces/{namespace}/pods/{name}/{name}.json`
//
// ### Config ID
// `conditional/pod_definition`
//
// ### Released version
// - 4.11.0
//
// ### Backported versions
// None
//
// ### Changes
// - Modified to accept an optional 'pod_prefix' parameter, which allows multiple pods to be gathered under the same alert's namespace by prefix.
func (g *Gatherer) BuildGatherPodDefinition(paramsInterface interface{}) (gatherers.GatheringClosure, error) { // nolint: dupl
	params, ok := paramsInterface.(GatherPodDefinitionParams)
	if !ok {
		return gatherers.GatheringClosure{}, fmt.Errorf(
			"unexpected type in paramsInterface, expected %T, got %T",
			GatherContainersLogsParams{},
			paramsInterface)
	}

	return gatherers.GatheringClosure{
		Run: func(ctx context.Context) ([]record.Record, []error) {
			kubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
			if err != nil {
				return nil, []error{err}
			}
			coreClient := kubeClient.CoreV1()
			return g.gatherPodDefinition(ctx, params, coreClient)
		},
	}, nil
}

func (g *Gatherer) gatherPodDefinition(
	ctx context.Context,
	params GatherPodDefinitionParams,
	coreClient corev1client.CoreV1Interface,
) ([]record.Record, []error) {
	alertInstances, ok := g.firingAlerts[params.AlertName]
	if !ok {
		err := fmt.Errorf("conditional gather triggered, but specified alert %q is not firing", params.AlertName)
		return nil, []error{err}
	}

	const logMissingAlert = "%s at alertName: %s"

	var errs []error
	var records []record.Record

	for _, alertLabels := range alertInstances {
		podNamespace, err := getAlertPodNamespace(alertLabels)
		if err != nil {
			klog.Warningf(logMissingAlert, err.Error(), params.AlertName)
			errs = append(errs, err)
			continue
		}

		var podDefinitions []*v1.Pod
		if len(params.PodPrefix) > 0 {
			// New logic has been introduced to retrieve a list of pods with a given prefix (from params)
			podList, err := coreClient.Pods(podNamespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				klog.Errorf("failed to list pods in namespace %s: %v", podNamespace, err)
				return nil, []error{err}
			}

			podDefinitions = filterPodsByPrefix(podList, params.PodPrefix)

		} else {
			// Previous logic to retrieve ONLY the pod definition from the firing alert
			podName, err := getAlertPodName(alertLabels)
			if err != nil {
				klog.Warningf(logMissingAlert, err.Error(), params.AlertName)
				errs = append(errs, err)
				continue
			}

			pod, err := coreClient.Pods(podNamespace).Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				klog.Warningf("pod %s not found in %s namespace: %v", podName, podNamespace, err)
				errs = append(errs, err)
				continue
			}
			anonymize.SensitiveEnvVars(pod.Spec.Containers)

			podDefinitions = []*v1.Pod{pod}
		}

		for i := range podDefinitions {
			pod := podDefinitions[i]
			podName := pod.GetName()

			records = append(records, record.Record{
				Name: fmt.Sprintf(
					"%s/namespaces/%s/pods/%s/%s",
					g.GetName(),
					podNamespace,
					podName,
					podName),
				Item: record.ResourceMarshaller{Resource: pod},
			})
		}
	}

	return records, errs
}

func filterPodsByPrefix(podList *v1.PodList, prefix string) (filteredList []*v1.Pod) {
	for i := range podList.Items {
		pod := &podList.Items[i]
		if !strings.HasPrefix(pod.Name, prefix) {
			continue
		}

		anonymize.SensitiveEnvVars(pod.Spec.Containers)

		filteredList = append(filteredList, pod)
	}

	return
}
