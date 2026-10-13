package clusterconfig

import (
	"context"
	"fmt"

	v1 "k8s.io/client-go/kubernetes/typed/storage/v1"

	"github.com/openshift/insights-operator/pkg/record"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// GatherStorageClasses Collects the cluster `StorageClass` available in cluster.
//
// ### API Reference
// - https://docs.openshift.com/container-platform/4.13/rest_api/storage_apis/storageclass-storage-k8s-io-v1.html
//
// ### Sample data
// - docs/insights-archive-sample/config/storage/storageclasses/standard-csi.json
//
// ### Location in archive
// - `config/storage/storageclasses/{name}.json`
//
// ### Config ID
// `clusterconfig/storage_classes`
//
// ### Released version
// - 4.15
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherStorageClasses(ctx context.Context) ([]record.Record, []error) {
	kubeClient, err := kubernetes.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherStorageClasses(ctx, kubeClient.StorageV1())
}

func gatherStorageClasses(ctx context.Context, storageClient v1.StorageV1Interface) ([]record.Record, []error) {
	storageClasses, err := listStorageClasses(ctx, storageClient.StorageClasses())
	if err != nil {
		return nil, []error{err}
	}

	var records []record.Record
	for i := range storageClasses.Items {
		item := &storageClasses.Items[i]
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/storage/storageclasses/%s", item.GetName()),
			Item: record.ResourceMarshaller{Resource: item},
		})
	}

	return records, nil
}

func listStorageClasses(ctx context.Context, storageClient v1.StorageClassInterface) (*storagev1.StorageClassList, error) {
	storageClasses, err := storageClient.List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return storageClasses, nil
}
