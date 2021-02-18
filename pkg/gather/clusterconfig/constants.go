package clusterconfig

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	datahubGroupVersionResource = schema.GroupVersionResource{
		Group: "installers.datahub.sap.com", Version: "v1alpha1", Resource: "datahubs",
	}
)
