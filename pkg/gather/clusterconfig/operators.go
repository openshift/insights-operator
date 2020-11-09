package clusterconfig

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"

	configv1 "github.com/openshift/api/config/v1"
	_ "k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/record/diskrecorder"
)

const (
	// Log compression ratio is defining a multiplier for uncompressed logs
	// diskrecorder would refuse to write files larger than MaxLogSize, so GatherClusterOperators
	// has to limit the expected size of the buffer for logs
	logCompressionRatio = 2
)

// GatherClusterOperators collects all ClusterOperators.
// It finds unhealthy Pods for unhealthy operators
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/clusteroperator.go#L62
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#clusteroperatorlist-v1config-openshift-io
//
// Location of operators in archive: config/clusteroperator/
// See: docs/insights-archive-sample/config/clusteroperator
// Location of pods in archive: config/pod/
func GatherClusterOperators(g *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		config, err := g.client.ClusterOperators().List(g.ctx, metav1.ListOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		resVer, _ := getOperatorResourcesVersions(g)
		records := make([]record.Record, 0, len(config.Items))
		for _, co := range config.Items {
			records = append(records, record.Record{
				Name: fmt.Sprintf("config/clusteroperator/%s", co.Name),
				Item: ClusterOperatorAnonymizer{&co},
			})
			if resVer == nil {
				continue
			}
			relRes := collectClusterOperatorResources(g, co, resVer)
			for _, rr := range relRes {
				records = append(records, record.Record{
					Name: fmt.Sprintf("config/clusteroperator/%s-%s", co.Name, rr.Name),
					Item: record.JSONMarshaller{Object: rr},
				})
			}
		}
		namespaceEventsCollected := sets.NewString()
		now := time.Now()
		unhealthyPods := []*corev1.Pod{}
		for _, item := range config.Items {
			if isHealthyOperator(&item) {
				continue
			}
			for _, namespace := range namespacesForOperator(&item) {
				pods, err := g.coreClient.Pods(namespace).List(g.ctx, metav1.ListOptions{})
				if err != nil {
					klog.V(2).Infof("Unable to find pods in namespace %s for failing operator %s", namespace, item.Name)
					continue
				}
				for j := range pods.Items {
					pod := &pods.Items[j]
					if isHealthyPod(pod, now) {
						continue
					}
					records = append(records, record.Record{Name: fmt.Sprintf("config/pod/%s/%s", pod.Namespace, pod.Name), Item: PodAnonymizer{pod}})
					unhealthyPods = append(unhealthyPods, pod)
				}
				if namespaceEventsCollected.Has(namespace) {
					continue
				}
				namespaceRecords, errs := g.gatherNamespaceEvents(namespace)
				if len(errs) > 0 {
					klog.V(2).Infof("Unable to collect events for namespace %q: %#v", namespace, errs)
					continue
				}
				records = append(records, namespaceRecords...)
				namespaceEventsCollected.Insert(namespace)
			}
		}

		// Exit early if no unhealthy pods found
		if len(unhealthyPods) == 0 {
			return records, nil
		}

		// Fetch a list of containers in unhealthy pods and calculate a log size quota
		// Total log size must not exceed maxLogsSize multiplied by logCompressionRatio
		klog.V(2).Infof("Found %d unhealthy pods", len(unhealthyPods))
		totalUnhealthyContainers := 0
		for _, pod := range unhealthyPods {
			totalUnhealthyContainers += len(pod.Spec.InitContainers) + len(pod.Spec.Containers)
		}
		bufferSize := int64(diskrecorder.MaxLogSize * logCompressionRatio / totalUnhealthyContainers / 2)
		klog.V(2).Infof("Maximum buffer size: %v bytes", bufferSize)
		buf := bytes.NewBuffer(make([]byte, 0, bufferSize))

		// Fetch previous and current container logs
		for _, isPrevious := range []bool{true, false} {
			for _, pod := range unhealthyPods {
				allContainers := pod.Spec.InitContainers
				allContainers = append(allContainers, pod.Spec.Containers...)
				for _, c := range allContainers {
					logName := fmt.Sprintf("%s_current.log", c.Name)
					if isPrevious {
						logName = fmt.Sprintf("%s_previous.log", c.Name)
					}
					buf.Reset()
					klog.V(2).Infof("Fetching logs for %s container %s pod in namespace %s (previous: %v): %v", c.Name, pod.Name, pod.Namespace, isPrevious, err)
					// Collect container logs and continue on error
					err = collectContainerLogs(g, pod, buf, c.Name, isPrevious, &bufferSize)
					if err != nil {
						klog.V(2).Infof("Error: %q", err)
						continue
					}
					records = append(records, record.Record{Name: fmt.Sprintf("config/pod/%s/logs/%s/%s", pod.Namespace, pod.Name, logName), Item: Raw{buf.String()}})
				}
			}
		}

		return records, nil
	}
}

func collectClusterOperatorResources(g *Gatherer, co configv1.ClusterOperator, resVer map[string][]string) []clusterOperatorResource {
	var relObj []configv1.ObjectReference
	for _, ro := range co.Status.RelatedObjects {
		if strings.Contains(ro.Group, "operator.openshift.io") {
			relObj = append(relObj, ro)
		}
	}
	if len(relObj) == 0 {
		return nil
	}
	var res []clusterOperatorResource
	for _, ro := range relObj {
		key := fmt.Sprintf("%s-%s", ro.Group, strings.ToLower(ro.Resource))
		versions, _ := resVer[key]
		for _, v := range versions {
			gvr := schema.GroupVersionResource{Group: ro.Group, Version: v, Resource: strings.ToLower(ro.Resource)}
			clusterResource, err := g.dynamicClient.Resource(gvr).Get(g.ctx, ro.Name, metav1.GetOptions{})
			if err != nil {
				klog.V(2).Infof("Unable to list %s resource due to: %s", gvr, err)
			}
			if clusterResource == nil {
				return nil
			}
			var ms, kind, name, apiVersion string
			err = failEarly(
				func() error { return parseJSONQuery(clusterResource.Object, "spec.managementState?", &ms) },
				func() error { return parseJSONQuery(clusterResource.Object, "kind", &kind) },
				func() error { return parseJSONQuery(clusterResource.Object, "apiVersion", &apiVersion) },
				func() error { return parseJSONQuery(clusterResource.Object, "metadata.name", &name) },
			)
			if err != nil {
				return nil
			}
			res = append(res, clusterOperatorResource{ManagementState: ms, Kind: kind, Name: name, APIVersion: apiVersion})
		}
	}
	return res
}

func getOperatorResourcesVersions(g *Gatherer) (map[string][]string, error) {
	resources, err := g.discoveryClient.ServerPreferredResources()
	if err != nil {
		return nil, err
	}
	resourceVersionMap := make(map[string][]string)
	for _, v := range resources {
		if strings.Contains(v.GroupVersion, "operator.openshift.io") {
			gv, err := schema.ParseGroupVersion(v.GroupVersion)
			if err != nil {
				continue
			}
			for _, ar := range v.APIResources {
				key := fmt.Sprintf("%s-%s", gv.Group, ar.Name)
				r, ok := resourceVersionMap[key]
				if !ok {
					resourceVersionMap[key] = []string{gv.Version}
				} else {
					r = append(r, gv.Version)
				}
			}
		}
	}
	return resourceVersionMap, nil
}

// collectContainerLogs fetches log lines from the pod
func collectContainerLogs(g *Gatherer, pod *corev1.Pod, buf *bytes.Buffer, containerName string, isPrevious bool, maxBytes *int64) error {
	req := g.coreClient.Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{Previous: isPrevious, Container: containerName, LimitBytes: maxBytes, TailLines: &logTailLines})
	readCloser, err := req.Stream(g.ctx)
	if err != nil {
		klog.V(2).Infof("Failed to fetch log for %s pod in namespace %s for failing operator %s (previous: %v): %q", pod.Name, pod.Namespace, containerName, isPrevious, err)
		return err
	}

	defer readCloser.Close()

	_, err = io.Copy(buf, readCloser)
	if err != nil && err != io.ErrShortBuffer {
		klog.V(2).Infof("Failed to write log for %s pod in namespace %s for failing operator %s (previous: %v): %q", pod.Name, pod.Namespace, containerName, isPrevious, err)
		return err
	}
	return nil
}

// ClusterOperatorAnonymizer implements serialization of ClusterOperator without change
type ClusterOperatorAnonymizer struct{ *configv1.ClusterOperator }

// Marshal serializes ClusterOperator
func (a ClusterOperatorAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(openshiftSerializer, a.ClusterOperator)
}

// GetExtension returns extension for anonymized cluster operator objects
func (a ClusterOperatorAnonymizer) GetExtension() string {
	return "json"
}

func isHealthyOperator(operator *configv1.ClusterOperator) bool {
	for _, condition := range operator.Status.Conditions {
		switch {
		case condition.Type == configv1.OperatorDegraded && condition.Status == configv1.ConditionTrue,
			condition.Type == configv1.OperatorAvailable && condition.Status == configv1.ConditionFalse:
			return false
		}
	}
	return true
}

func namespacesForOperator(operator *configv1.ClusterOperator) []string {
	var ns []string
	for _, ref := range operator.Status.RelatedObjects {
		if ref.Resource == "namespaces" {
			ns = append(ns, ref.Name)
		}
	}
	return ns
}
