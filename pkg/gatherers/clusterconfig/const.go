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
	// logMaxLines sets maximum number of lines of the log file
	logMaxTailLines = int64(100)
	// logMaxLongTailLines sets maximum number of lines of the long log file
	logMaxLongTailLines = int64(2000)
	// logLinesOffset sets the maximum offset if a stacktrace message was found in the logs
	logLinesOffset = 20

	defaultNamespaces           = []string{"default", "kube-system", "kube-public", "openshift"}
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
		Group: "machine.openshift.io", Version: "v1beta1", Resource: "machinehealthchecks",
	}
	machineAutoScalerGvr = schema.GroupVersionResource{
		Group: "autoscaling.openshift.io", Version: "v1beta1", Resource: "machineautoscalers",
	}
)

func init() { //nolint: gochecknoinits
	utilruntime.Must(registryv1.AddToScheme(registryScheme))
}
