package controller

import (
	"fmt"
	"os"

	"github.com/openshift/insights-operator/pkg/config/configobserver"
)

// getCustomStoragePath determines a custom storage path by checking configuration sources
// * ConfigMap configuration (DataReporting.StoragePath)
func getCustomStoragePath(configAggregator configobserver.Interface) string {
	defaultPath := ""

	// Get the default path from ConfigMap configuration
	if configStoragePath := configAggregator.Config().DataReporting.StoragePath; configStoragePath != "" {
		defaultPath = configStoragePath
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
