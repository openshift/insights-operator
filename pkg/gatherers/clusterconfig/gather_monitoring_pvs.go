package clusterconfig

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift/insights-operator/pkg/record"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	coreV1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

// GatherMonitoringPVs Collects Persistent Volumes from openshift-monitoring namespace
// which matches with ConfigMap configuration yaml
//
// ### API Reference
// - https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/configmap.go
// - https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/persistentvolume.go
//
// ### Sample data
// - docs/insights-archive-sample/config/persistentvolumes/monitoring-persistent-volume.json
//
// ### Location in archive
// - `config/persistentvolumes/{persistent_volume_name}.json`
//
// ### Config ID
// `clusterconfig/monitoring_persistent_volumes`
//
// ### Released version
// - 4.14
//
// ### Changes
// None
func (g *Gatherer) GatherMonitoringPVs(ctx context.Context) ([]record.Record, []error) {
	kubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	mg := MonitoringPVGatherer{client: kubeClient.CoreV1()}

	return mg.gather(ctx)
}

type MonitoringPVGatherer struct {
	client coreV1.CoreV1Interface
}

// gather returns the persistent volumes found as records for its gathering
// and a collection of errors
func (mg MonitoringPVGatherer) gather(ctx context.Context) ([]record.Record, []error) {
	const NAMESPACE = "openshift-monitoring"
	const PROMETHEUS_DEFAULT = "prometheus-k8s"

	pvcList, err := mg.client.PersistentVolumeClaims(NAMESPACE).List(ctx, metaV1.ListOptions{})
	if err != nil {
		return []record.Record{}, []error{err}
	}

	var records []record.Record
	var errors []error

	pvInterface := mg.client.PersistentVolumes()
	for i := range pvcList.Items {
		pvcName := pvcList.Items[i].Name

		if strings.Contains(pvcName, PROMETHEUS_DEFAULT) {
			pvName := pvcList.Items[i].Spec.VolumeName

			pv, err := pvInterface.Get(ctx, pvName, metaV1.GetOptions{})
			if err != nil {
				errors = append(errors, err)
				continue
			}

			records = append(records, record.Record{
				Name: fmt.Sprintf("config/persistentvolumes/%s", pv.Name),
				Item: record.ResourceMarshaller{Resource: pv},
			})
		}
	}

	return records, errors
}
