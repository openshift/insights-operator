package conditional

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/record"
)

// BuildGatherPodDefinition collects pod definition from pods that are firing one of the configured alerts.
//
// * Location in archive: conditional/namespaces/<namespace>/pods/<pod>.json
// * Id in config: pod_definition
// * Since versions:
//   * 4.10+
func (g *Gatherer) BuildGatherPodDefinition(paramsInterface interface{}) (gatherers.GatheringClosure, error) {
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
		podName, err := getAlertPodName(alertLabels)
		if err != nil {
			klog.Warningf(logMissingAlert, err.Error(), params.AlertName)
			errs = append(errs, err)
			continue
		}

		pod, err := coreClient.Pods(podNamespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			klog.Warningf("pod %s not found in %s namespace: %w", podName, podNamespace, err)
			errs = append(errs, err)
			continue
		}

		records = append(records, record.Record{
			Name: fmt.Sprintf(
				"%s/namespaces/%s/pods/%s",
				g.GetName(),
				podNamespace,
				podName),
			Item: record.ResourceMarshaller{Resource: pod},
		})
	}

	return records, errs
}
