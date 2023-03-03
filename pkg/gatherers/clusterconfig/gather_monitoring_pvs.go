package clusterconfig

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	coreV1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"sigs.k8s.io/yaml"
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
// ### Backported versions (tentative)
// - +4.13.0
// - +4.12.6
// - +4.11.30
//
// ### Changes
// None
func (g *Gatherer) GatherMonitoringPVs(ctx context.Context) ([]record.Record, []error) {
	kubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	mg := MonitoringPVGatherer{client: kubeClient.CoreV1()}

	name, err := mg.getDefaultPrometheusName(ctx)
	if err != nil {
		return nil, []error{err}
	}

	return mg.gather(ctx, name)
}

type MonitoringPVGatherer struct {
	client coreV1.CoreV1Interface
}

// getDefaultPrometheusName returns prometheus name as it's described on the configmap
// or an error collection from the attempts to retrieve that information
func (mg MonitoringPVGatherer) getDefaultPrometheusName(ctx context.Context) (string, error) {
	const CMO = "cluster-monitoring-config"
	const NAMESPACE = "openshift-monitoring"
	const CONFIG = "config.yaml"

	cm, err := mg.client.ConfigMaps(NAMESPACE).Get(ctx, CMO, metaV1.GetOptions{})
	if err != nil {
		return "", err
	}

	rawData, exists := cm.Data[CONFIG]
	if !exists {
		return "", fmt.Errorf("no %s data on %s ConfigMap", CONFIG, CMO)
	}

	name, err := mg.unmarshalDefaultPath(rawData)
	if err != nil {
		return "", err
	}

	return name, nil
}

// unmarshalDefaultPath returns prometheus name from a given raw data (yaml format)
// or an error if the raw data is not unmarshalable or it lacks the default path
func (mg MonitoringPVGatherer) unmarshalDefaultPath(raw string) (string, error) {
	var defaultPath = []string{"prometheusK8s", "volumeClaimTemplate", "metadata", "name"}
	var configYaml map[string]interface{}

	err := yaml.Unmarshal([]byte(raw), &configYaml)
	if err != nil {
		return "", err
	}

	target, err := utils.NestedStringWrapper(configYaml, defaultPath...)
	if err != nil {
		return "", err
	}

	return target, nil
}

// gather returns the persistent volumes found as records for its gathering
// and a collection of errors
func (mg MonitoringPVGatherer) gather(ctx context.Context, prefix string) ([]record.Record, []error) {
	const NAMESPACE = "openshift-monitoring"

	pvcList, err := mg.client.PersistentVolumeClaims(NAMESPACE).List(ctx, metaV1.ListOptions{})
	if err != nil {
		return []record.Record{}, []error{err}
	}

	var records []record.Record
	var errors []error

	pvInterface := mg.client.PersistentVolumes()
	for i := range pvcList.Items {
		pvcName := pvcList.Items[i].Name

		if strings.HasPrefix(pvcName, prefix) {
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
