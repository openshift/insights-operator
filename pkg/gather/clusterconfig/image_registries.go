package clusterconfig

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"

	registryv1 "github.com/openshift/api/imageregistry/v1"
	imageregistryv1client "github.com/openshift/client-go/imageregistry/clientset/versioned"
	imageregistryv1 "github.com/openshift/client-go/imageregistry/clientset/versioned/typed/imageregistry/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

var lacAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

// GatherClusterImageRegistry fetches the cluster Image Registry configuration
// If the Image Registry configuration uses some PersistentVolumeClaim for the storage then the corresponding
// PersistentVolume definition is gathered
//
// Location in archive: config/clusteroperator/imageregistry.operator.openshift.io/config/cluster.json
// Id in config: image_registries
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

// ImageRegistryAnonymizer implements serialization with marshalling
type ImageRegistryAnonymizer struct {
	*registryv1.Config
}

// Marshal implements serialization of Ingres.Spec.Domain with anonymization
func (a ImageRegistryAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	a.Spec.HTTPSecret = anonymizeString(a.Spec.HTTPSecret)
	if a.Spec.Storage.S3 != nil {
		anonymizeS3Storage(a.Spec.Storage.S3)
	}
	if a.Spec.Storage.Azure != nil {
		anonymizeAzureStorage(a.Spec.Storage.Azure)
	}
	if a.Spec.Storage.GCS != nil {
		anonymizeGCSStorage(a.Spec.Storage.GCS)
	}
	if a.Spec.Storage.Swift != nil {
		anonymizeSwiftStorage(a.Spec.Storage.Swift)
	}
	if a.Status.Storage.S3 != nil {
		anonymizeS3Storage(a.Status.Storage.S3)
	}
	if a.Status.Storage.GCS != nil {
		anonymizeGCSStorage(a.Status.Storage.GCS)
	}
	if a.Status.Storage.Azure != nil {
		anonymizeAzureStorage(a.Status.Storage.Azure)
	}
	if a.Status.Storage.Swift != nil {
		anonymizeSwiftStorage(a.Status.Storage.Swift)
	}
	// kubectl.kubernetes.io/last-applied-configuration annotation contains complete previous resource definition
	// including the sensitive information as bucket, keyIDs, etc.
	if lac, ok := a.Annotations[lacAnnotation]; ok {
		a.Annotations[lacAnnotation] = anonymizeString(lac)
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

// PersistentVolumeAnonymizer implements serialization with marshalling
type PersistentVolumeAnonymizer struct {
	*corev1.PersistentVolume
}

// Marshal implements serialization of corev1.PersistentVolume without anonymization
func (p PersistentVolumeAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(kubeSerializer, p.PersistentVolume)
}

// GetExtension returns extension for PersistentVolume objects
func (p PersistentVolumeAnonymizer) GetExtension() string {
	return "json"
}

func anonymizeS3Storage(s3Storage *registryv1.ImageRegistryConfigStorageS3) {
	s3Storage.Bucket = anonymizeString(s3Storage.Bucket)
	s3Storage.KeyID = anonymizeString(s3Storage.KeyID)
	s3Storage.RegionEndpoint = anonymizeString(s3Storage.RegionEndpoint)
	s3Storage.Region = anonymizeString(s3Storage.Region)
}

func anonymizeGCSStorage(gcsStorage *registryv1.ImageRegistryConfigStorageGCS) {
	gcsStorage.Bucket = anonymizeString(gcsStorage.Bucket)
	gcsStorage.KeyID = anonymizeString(gcsStorage.KeyID)
	gcsStorage.ProjectID = anonymizeString(gcsStorage.ProjectID)
	gcsStorage.Region = anonymizeString(gcsStorage.Region)
}

func anonymizeAzureStorage(azureStorage *registryv1.ImageRegistryConfigStorageAzure) {
	azureStorage.AccountName = anonymizeString(azureStorage.AccountName)
	azureStorage.Container = anonymizeString(azureStorage.Container)
	azureStorage.CloudName = anonymizeString(azureStorage.CloudName)
}

func anonymizeSwiftStorage(swiftStorage *registryv1.ImageRegistryConfigStorageSwift) {
	swiftStorage.AuthURL = anonymizeString(swiftStorage.AuthURL)
	swiftStorage.Container = anonymizeString(swiftStorage.Container)
	swiftStorage.Domain = anonymizeString(swiftStorage.Domain)
	swiftStorage.DomainID = anonymizeString(swiftStorage.DomainID)
	swiftStorage.Tenant = anonymizeString(swiftStorage.Tenant)
	swiftStorage.TenantID = anonymizeString(swiftStorage.TenantID)
	swiftStorage.RegionName = anonymizeString(swiftStorage.RegionName)
}
