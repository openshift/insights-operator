package clusterconfig

import (
	"context"
	"fmt"
	"strings"

	imageregistryv1client "github.com/openshift/client-go/imageregistry/clientset/versioned"
	imageregistryv1 "github.com/openshift/client-go/imageregistry/clientset/versioned/typed/imageregistry/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"

	registryv1 "github.com/openshift/api/imageregistry/v1"

	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

// ImageRegistryAnonymizer implements serialization with marshalling
type ImageRegistryAnonymizer struct {
	*registryv1.Config
}

// PersistentVolumeAnonymizer implements serialization with marshalling
type PersistentVolumeAnonymizer struct {
	*corev1.PersistentVolume
}

// GatherClusterImageRegistry fetches the cluster Image Registry configuration
// If the Image Registry configuration uses some PersistentVolumeClaim for the storage then the corresponding
// PersistentVolume definition is gathered
//
// Location in archive: config/clusteroperator/imageregistry.operator.openshift.io/config/cluster.json
func GatherClusterImageRegistry(g *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		registryClient, err := imageregistryv1client.NewForConfig(g.gatherKubeConfig)
		if err != nil {
			return nil, []error{err}
		}
		gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
		if err != nil {
			return nil, []error{err}
		}
		return gatherClusterImageRegistry(g.ctx, registryClient.ImageregistryV1(), gatherKubeClient.CoreV1())
	}
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
				Item: PersistentVolumeAnonymizer{pv},
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
		Item: ImageRegistryAnonymizer{config},
	}

	records = append(records, coRecord)
	return records, nil
}

// Marshal implements serialization of Ingres.Spec.Domain with anonymization
func (a ImageRegistryAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	a.Spec.HTTPSecret = anonymizeString(a.Spec.HTTPSecret)
	if a.Spec.Storage.S3 != nil {
		a.Spec.Storage.S3.Bucket = anonymizeString(a.Spec.Storage.S3.Bucket)
		a.Spec.Storage.S3.KeyID = anonymizeString(a.Spec.Storage.S3.KeyID)
		a.Spec.Storage.S3.RegionEndpoint = anonymizeString(a.Spec.Storage.S3.RegionEndpoint)
		a.Spec.Storage.S3.Region = anonymizeString(a.Spec.Storage.S3.Region)
	}
	if a.Spec.Storage.Azure != nil {
		a.Spec.Storage.Azure.AccountName = anonymizeString(a.Spec.Storage.Azure.AccountName)
		a.Spec.Storage.Azure.Container = anonymizeString(a.Spec.Storage.Azure.Container)
	}
	if a.Spec.Storage.GCS != nil {
		a.Spec.Storage.GCS.Bucket = anonymizeString(a.Spec.Storage.GCS.Bucket)
		a.Spec.Storage.GCS.ProjectID = anonymizeString(a.Spec.Storage.GCS.ProjectID)
		a.Spec.Storage.GCS.KeyID = anonymizeString(a.Spec.Storage.GCS.KeyID)
	}
	if a.Spec.Storage.Swift != nil {
		a.Spec.Storage.Swift.AuthURL = anonymizeString(a.Spec.Storage.Swift.AuthURL)
		a.Spec.Storage.Swift.Container = anonymizeString(a.Spec.Storage.Swift.Container)
		a.Spec.Storage.Swift.Domain = anonymizeString(a.Spec.Storage.Swift.Domain)
		a.Spec.Storage.Swift.DomainID = anonymizeString(a.Spec.Storage.Swift.DomainID)
		a.Spec.Storage.Swift.Tenant = anonymizeString(a.Spec.Storage.Swift.Tenant)
		a.Spec.Storage.Swift.TenantID = anonymizeString(a.Spec.Storage.Swift.TenantID)
		a.Spec.Storage.Swift.RegionName = anonymizeString(a.Spec.Storage.Swift.RegionName)
	}
	return runtime.Encode(registrySerializer.LegacyCodec(registryv1.SchemeGroupVersion), a.Config)
}

// GetExtension returns extension for anonymized image registry objects
func (a ImageRegistryAnonymizer) GetExtension() string {
	return "json"
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

// Marshal implements serialization of corev1.PersistentVolume without anonymization
func (p PersistentVolumeAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(kubeSerializer, p.PersistentVolume)
}

// GetExtension returns extension for PersistentVolume objects
func (p PersistentVolumeAnonymizer) GetExtension() string {
	return "json"
}
