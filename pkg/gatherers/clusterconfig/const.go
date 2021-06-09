package clusterconfig

import (
	registryv1 "github.com/openshift/api/imageregistry/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

const (
	jsonExtension = "json"
)

var (
	registryScheme = runtime.NewScheme()
	// logTailLines sets maximum number of lines to fetch from pod logs
	logTailLines = int64(100)
	// logTailLinesLong sets the maximum number of lines to fetch from long pod logs
	logTailLinesLong = int64(400)

	defaultNamespaces           = []string{"default", "kube-system", "kube-public"}
	datahubGroupVersionResource = schema.GroupVersionResource{
		Group: "installers.datahub.sap.com", Version: "v1alpha1", Resource: "datahubs",
	}
	pnccGroupVersionResource = schema.GroupVersionResource{
		Group: "controlplane.operator.openshift.io", Version: "v1alpha1", Resource: "podnetworkconnectivitychecks",
	}
	machineConfigGroupVersionResource = schema.GroupVersionResource{
		Group: "machineconfiguration.openshift.io", Version: "v1", Resource: "machineconfigs",
	}
	machineHeatlhCheckGVR = schema.GroupVersionResource{
		Group: "machine.openshift.io", Version: "v1beta1", Resource: "machinehealthchecks"}
)

func init() { //nolint: gochecknoinits
	utilruntime.Must(registryv1.AddToScheme(registryScheme))
}
