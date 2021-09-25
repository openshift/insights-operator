package clusterconfig

import (
	"context"
	"fmt"
	"strings"

	registryv1 "github.com/openshift/api/imageregistry/v1"
	imageregistryv1client "github.com/openshift/client-go/imageregistry/clientset/versioned"
	imageregistryv1 "github.com/openshift/client-go/imageregistry/clientset/versioned/typed/imageregistry/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
)

var lacAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

// GatherClusterImageRegistry fetches the cluster Image Registry configuration
//
// **Conditional data**: If the Image Registry configuration uses any PersistentVolumeClaim for the storage, the corresponding
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
func (g *Gatherer) GatherClusterImageRegistry(ctx context.Context) ([]record.Record, []error) {
	registryClient, err := imageregistryv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherClusterImageRegistry(ctx, registryClient.ImageregistryV1(), gatherKubeClient.CoreV1())
}

//nolint: govet
func gatherClusterImageRegistry(ctx context.Context,
	registryClient imageregistryv1.ImageregistryV1Interface,
	coreClient corev1client.CoreV1Interface) ([]record.Record, []error) {
	config, err := registryClient.Configs().Get(ctx, "cluster", metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	var records []record.Record
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
	var pvc corev1.PersistentVolumeClaim
	for i := range pvcs.Items {
		if pvcs.Items[i].Name == name {
			pvc = pvcs.Items[i]
			break
		}
	}
	if pvc.Name == "" {
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
	config.Spec.HTTPSecret = anonymize.String(config.Spec.HTTPSecret)
	if config.Spec.Storage.S3 != nil {
		anonymizeS3Storage(config.Spec.Storage.S3)
	}
	if config.Spec.Storage.Azure != nil {
		anonymizeAzureStorage(config.Spec.Storage.Azure)
	}
	if config.Spec.Storage.GCS != nil {
		anonymizeGCSStorage(config.Spec.Storage.GCS)
	}
	if config.Spec.Storage.Swift != nil {
		anonymizeSwiftStorage(config.Spec.Storage.Swift)
	}
	if config.Spec.Storage.IBMCOS != nil {
		anonymizeIBMCOSStorage(config.Spec.Storage.IBMCOS)
	}
	if config.Status.Storage.S3 != nil {
		anonymizeS3Storage(config.Status.Storage.S3)
	}
	if config.Status.Storage.GCS != nil {
		anonymizeGCSStorage(config.Status.Storage.GCS)
	}
	if config.Status.Storage.Azure != nil {
		anonymizeAzureStorage(config.Status.Storage.Azure)
	}
	if config.Status.Storage.Swift != nil {
		anonymizeSwiftStorage(config.Status.Storage.Swift)
	}
	if config.Status.Storage.IBMCOS != nil {
		anonymizeIBMCOSStorage(config.Status.Storage.IBMCOS)
	}
	// kubectl.kubernetes.io/last-applied-configuration annotation contains complete previous resource definition
	// including the sensitive information as bucket, keyIDs, etc.
	if lac, ok := config.Annotations[lacAnnotation]; ok {
		config.Annotations[lacAnnotation] = anonymize.String(lac)
	}

	return config
}

func anonymizeS3Storage(s3Storage *registryv1.ImageRegistryConfigStorageS3) {
	s3Storage.Bucket = anonymize.String(s3Storage.Bucket)
	s3Storage.KeyID = anonymize.String(s3Storage.KeyID)
	s3Storage.RegionEndpoint = anonymize.String(s3Storage.RegionEndpoint)
	s3Storage.Region = anonymize.String(s3Storage.Region)
}

func anonymizeGCSStorage(gcsStorage *registryv1.ImageRegistryConfigStorageGCS) {
	gcsStorage.Bucket = anonymize.String(gcsStorage.Bucket)
	gcsStorage.KeyID = anonymize.String(gcsStorage.KeyID)
	gcsStorage.ProjectID = anonymize.String(gcsStorage.ProjectID)
	gcsStorage.Region = anonymize.String(gcsStorage.Region)
}

func anonymizeAzureStorage(azureStorage *registryv1.ImageRegistryConfigStorageAzure) {
	azureStorage.AccountName = anonymize.String(azureStorage.AccountName)
	azureStorage.Container = anonymize.String(azureStorage.Container)
	azureStorage.CloudName = anonymize.String(azureStorage.CloudName)
}

func anonymizeSwiftStorage(swiftStorage *registryv1.ImageRegistryConfigStorageSwift) {
	swiftStorage.AuthURL = anonymize.String(swiftStorage.AuthURL)
	swiftStorage.Container = anonymize.String(swiftStorage.Container)
	swiftStorage.Domain = anonymize.String(swiftStorage.Domain)
	swiftStorage.DomainID = anonymize.String(swiftStorage.DomainID)
	swiftStorage.Tenant = anonymize.String(swiftStorage.Tenant)
	swiftStorage.TenantID = anonymize.String(swiftStorage.TenantID)
	swiftStorage.RegionName = anonymize.String(swiftStorage.RegionName)
}

func anonymizeIBMCOSStorage(ibmcosStorage *registryv1.ImageRegistryConfigStorageIBMCOS) {
	ibmcosStorage.Bucket = anonymize.String(ibmcosStorage.Bucket)
	ibmcosStorage.ResourceKeyCRN = anonymize.String(ibmcosStorage.ResourceKeyCRN)
	ibmcosStorage.ServiceInstanceCRN = anonymize.String(ibmcosStorage.ServiceInstanceCRN)
	ibmcosStorage.ResourceGroupName = anonymize.String(ibmcosStorage.ResourceGroupName)
	ibmcosStorage.Location = anonymize.String(ibmcosStorage.Location)
}
