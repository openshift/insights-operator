package controller

import (
	"fmt"
	"os"

	insightsv1alpha2 "github.com/openshift/api/insights/v1alpha2"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
)

// getCustomStoragePath determines a custom storage path by checking configuration sources
// in priority order:
// * DataGather CR specification (PersistentVolume.MountPath)
// * ConfigMap configuration (DataReporting.StoragePath)
func getCustomStoragePath(configAggregator configobserver.Interface, dataGatherCR *insightsv1alpha2.DataGather) string {
	defaultPath := ""

	// Get the default path from ConfigMap configuration
	if configStoragePath := configAggregator.Config().DataReporting.StoragePath; configStoragePath != "" {
		defaultPath = configStoragePath
	}

	if dataGatherCR == nil {
		return defaultPath
	}

	if dataGatherCR.Spec.Storage == nil || dataGatherCR.Spec.Storage.Type != insightsv1alpha2.StorageTypePersistentVolume {
		return defaultPath
	}

	if dataGatherCR.Spec.Storage.PersistentVolume != nil {
		if storagePath := dataGatherCR.Spec.Storage.PersistentVolume.MountPath; storagePath != "" {
			return storagePath
		}
	}

	return defaultPath
}

// pathIsAvailable
func pathIsAvailable(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0o777); err != nil {
			return false, fmt.Errorf("can't create --path: %v", err)
		}
	}

	return true, nil
}
