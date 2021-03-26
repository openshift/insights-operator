package clusterconfig

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"

	registryv1 "github.com/openshift/api/imageregistry/v1"
	imageregistryv1client "github.com/openshift/client-go/imageregistry/clientset/versioned"
	imageregistryv1 "github.com/openshift/client-go/imageregistry/clientset/versioned/typed/imageregistry/v1"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
)

// GatherClusterImageRegistry fetches the cluster Image Registry configuration
// If the Image Registry configuration uses some PersistentVolumeClaim for the storage then the corresponding
// PersistentVolume definition is gathered
//
// * Location in archive: config/clusteroperator/imageregistry.operator.openshift.io/config/cluster.json
// * Id in config: image_registries
// * Since versions:
//   * 4.3.40+
//   * 4.4.12+
//   * 4.5+
// * PV definition since versions:
//   * 4.6.20+
//   * 4.7+
func GatherClusterImageRegistry(g *Gatherer, c chan<- gatherResult) {
	defer close(c)
	registryClient, err := imageregistryv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}
	records, errors := gatherClusterImageRegistry(g.ctx, registryClient.ImageregistryV1(), gatherKubeClient.CoreV1())
	c <- gatherResult{records, errors}
}

func gatherClusterImageRegistry(ctx context.Context, registryClient imageregistryv1.ImageregistryV1Interface, coreClient corev1client.CoreV1Interface) ([]record.Record, []error) {
	config, err := registryClient.Configs().Get(ctx, "cluster", metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	records := []record.Record{}
	// if there is some PVC then try to gather used persistent volume
	if config.Spec.Storage.PVC != nil {

		pvcName := config.Spec.Storage.PVC.Claim
		pv, err := findPVByPVCName(ctx, coreClient, pvcName)
		if err != nil {
			klog.Errorf("unable to find persistent volume: %s", err)
		} else {
			pvRecord := record.Record{
				Name: fmt.Sprintf("config/persistentvolumes/%s", pv.Name),
				Item: record.JSONMarshaller{Object: pv},
			}
			records = append(records, pvRecord)
		}
	}
	// TypeMeta is empty - see https://github.com/kubernetes/kubernetes/issues/3030
	kinds, _, err := registryScheme.ObjectKinds(config)
	if err != nil {
		return nil, []error{err}
	}
	if len(kinds) > 1 {
		klog.Warningf("More kinds for image registry config operator resource %s", kinds)
	}
	objKind := kinds[0]
	coRecord := record.Record{
		Name: fmt.Sprintf("config/clusteroperator/%s/%s/%s", objKind.Group, strings.ToLower(objKind.Kind), config.Name),
		Item: record.JSONMarshaller{Object: anonymizeImageRegistry(config)},
	}
	records = append(records, coRecord)
	return records, nil
}

// findPVByPVCName tries to find *corev1.PersistentVolume used in PersistentVolumeClaim with provided name
func findPVByPVCName(ctx context.Context, coreClient corev1client.CoreV1Interface, name string) (*corev1.PersistentVolume, error) {
	// unfortunately we can't do "coreClient.PersistentVolumeClaims("").Get(ctx, name, ... )"
	pvcs, err := coreClient.PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var pvc *corev1.PersistentVolumeClaim
	for _, p := range pvcs.Items {
		if p.Name == name {
			pvc = &p
			break
		}
	}
	if pvc == nil {
		return nil, fmt.Errorf("can't find any %s persistentvolumeclaim", name)
	}
	pvName := pvc.Spec.VolumeName
	pv, err := coreClient.PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return pv, nil
}

func anonymizeImageRegistry(config *registryv1.Config) *registryv1.Config {
	config.Spec.HTTPSecret = anonymize.AnonymizeString(config.Spec.HTTPSecret)
	if config.Spec.Storage.S3 != nil {
		config.Spec.Storage.S3.Bucket = anonymize.AnonymizeString(config.Spec.Storage.S3.Bucket)
		config.Spec.Storage.S3.KeyID = anonymize.AnonymizeString(config.Spec.Storage.S3.KeyID)
		config.Spec.Storage.S3.RegionEndpoint = anonymize.AnonymizeString(config.Spec.Storage.S3.RegionEndpoint)
		config.Spec.Storage.S3.Region = anonymize.AnonymizeString(config.Spec.Storage.S3.Region)
	}
	if config.Spec.Storage.Azure != nil {
		config.Spec.Storage.Azure.AccountName = anonymize.AnonymizeString(config.Spec.Storage.Azure.AccountName)
		config.Spec.Storage.Azure.Container = anonymize.AnonymizeString(config.Spec.Storage.Azure.Container)
	}
	if config.Spec.Storage.GCS != nil {
		config.Spec.Storage.GCS.Bucket = anonymize.AnonymizeString(config.Spec.Storage.GCS.Bucket)
		config.Spec.Storage.GCS.ProjectID = anonymize.AnonymizeString(config.Spec.Storage.GCS.ProjectID)
		config.Spec.Storage.GCS.KeyID = anonymize.AnonymizeString(config.Spec.Storage.GCS.KeyID)
	}
	if config.Spec.Storage.Swift != nil {
		config.Spec.Storage.Swift.AuthURL = anonymize.AnonymizeString(config.Spec.Storage.Swift.AuthURL)
		config.Spec.Storage.Swift.Container = anonymize.AnonymizeString(config.Spec.Storage.Swift.Container)
		config.Spec.Storage.Swift.Domain = anonymize.AnonymizeString(config.Spec.Storage.Swift.Domain)
		config.Spec.Storage.Swift.DomainID = anonymize.AnonymizeString(config.Spec.Storage.Swift.DomainID)
		config.Spec.Storage.Swift.Tenant = anonymize.AnonymizeString(config.Spec.Storage.Swift.Tenant)
		config.Spec.Storage.Swift.TenantID = anonymize.AnonymizeString(config.Spec.Storage.Swift.TenantID)
		config.Spec.Storage.Swift.RegionName = anonymize.AnonymizeString(config.Spec.Storage.Swift.RegionName)
	}
	return config
}
