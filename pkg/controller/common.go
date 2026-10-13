package controller

import (
	"fmt"
	"os"

	insightsv1 "github.com/openshift/api/insights/v1"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
)

// getCustomStoragePath determines a custom storage path by checking configuration sources
// in priority order:
// * DataGather CR specification (PersistentVolume.MountPath)
// * ConfigMap configuration (DataReporting.StoragePath)
func getCustomStoragePath(configAggregator configobserver.Interface, dataGatherCR *insightsv1.DataGather) string {
	defaultPath := ""

	// Get the default path from ConfigMap configuration
	if configStoragePath := configAggregator.Config().DataReporting.StoragePath; configStoragePath != "" {
		defaultPath = configStoragePath
	}

	if dataGatherCR == nil {
		return defaultPath
	}

	if dataGatherCR.Spec.Storage == (insightsv1.Storage{}) || dataGatherCR.Spec.Storage.Type != insightsv1.StorageTypePersistentVolume {
		return defaultPath
	}

	if dataGatherCR.Spec.Storage.PersistentVolume != (insightsv1.PersistentVolumeConfig{}) {
		if storagePath := dataGatherCR.Spec.Storage.PersistentVolume.MountPath; storagePath != "" {
			return storagePath
		}
	}

	return defaultPath
}

// pathIsAvailable checks if the given path exists and is accessible.
// If the path does not exist, it attempts to create it (including all parent directories).
//
// Returns true if the path exists or was successfully created (is available)
// and false with an error if the path cannot be created.
func pathIsAvailable(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0o777); err != nil {
			return false, fmt.Errorf("can't create --path: %v", err)
		}
	}

	return true, nil
}
