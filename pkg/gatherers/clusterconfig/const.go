package clusterconfig

import (
	registryv1 "github.com/openshift/api/imageregistry/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

const (
	// introduced by GatherAggregatedInstances gatherer
	monitoringNamespace string = "openshift-monitoring"
)

var (
	registryScheme = runtime.NewScheme()
	// logMaxLines sets maximum number of lines of the log file
	logMaxTailLines = int64(100)
	// logMaxLongTailLines sets maximum number of lines of the long log file
	logMaxLongTailLines = int64(2000)
	// logLinesOffset sets the maximum offset if a stacktrace message was found in the logs
	logLinesOffset              = 20
	logStackTraceMaxLines       = 40
	logStackTraceBeginningLimit = 35
	logStackTraceEndLimit       = 5
	// logNodeUnit sets the journal unit to be used to collect the node logs (options: kubelet or crio)
	logNodeUnit = "kubelet"
	// logNodeMaxTailLines sets the maximum number of lines to be fetched
	logNodeMaxTailLines = 1000
	// logNodeMaxLines sets the maximum number of lines of the node log to be stored per node
	logNodeMaxLines = 50

	defaultNamespaces           = []string{"default", "kube-system", "kube-public", "openshift"}
	datahubGroupVersionResource = schema.GroupVersionResource{
		Group: "installers.datahub.sap.com", Version: "v1alpha1", Resource: "datahubs",
	}
	machinesGVR = schema.GroupVersionResource{
		Group: "machine.openshift.io", Version: "v1beta1", Resource: "machines",
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
	openshiftLoggingResource = schema.GroupVersionResource{
		Group: "logging.openshift.io", Version: "v1", Resource: "clusterloggings",
	}
	lokiStackResource = schema.GroupVersionResource{
		Group: "loki.grafana.com", Version: "v1", Resource: "lokistacks",
	}
	storageClusterResource = schema.GroupVersionResource{
		Group: "ocs.openshift.io", Version: "v1", Resource: "storageclusters",
	}
	cephClustereResource = schema.GroupVersionResource{
		Group: "ceph.rook.io", Version: "v1", Resource: "cephclusters",
	}
	jaegerResource = schema.GroupVersionResource{
		Group: "jaegertracing.io", Version: "v1", Resource: "jaegers",
	}
	nodeFeatureResource = schema.GroupVersionResource{
		Group: "nfd.k8s-sigs.io", Version: "v1alpha1", Resource: "nodefeatures",
	}
	costManagementMetricsConfigResource = schema.GroupVersionResource{
		Group: "costmanagement-metrics-cfg.openshift.io", Version: "v1beta1", Resource: "costmanagementmetricsconfigs",
	}
	operatorGVR = schema.GroupVersionResource{
		Group: "operators.coreos.com", Version: "v1", Resource: "operators",
	}
	clusterServiceVersionGVR = schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "clusterserviceversions",
	}
	oscpGroupVersionResource = schema.GroupVersionResource{
		Group: "core.openstack.org", Version: "v1beta1", Resource: "openstackcontrolplanes",
	}
	osdpdGroupVersionResource = schema.GroupVersionResource{
		Group: "dataplane.openstack.org", Version: "v1beta1", Resource: "openstackdataplanedeployments",
	}
	osdpnsGroupVersionResource = schema.GroupVersionResource{
		Group: "dataplane.openstack.org", Version: "v1beta1", Resource: "openstackdataplanenodesets",
	}
	osvGroupVersionResource = schema.GroupVersionResource{
		Group: "core.openstack.org", Version: "v1beta1", Resource: "openstackversions",
	}

	nodeNetConfPoliciesV1GVR = schema.GroupVersionResource{Group: "nmstate.io", Version: "v1", Resource: "nodenetworkconfigurationpolicies"}
	nodeNetStatesV1Beta1GVR  = schema.GroupVersionResource{Group: "nmstate.io", Version: "v1beta1", Resource: "nodenetworkstates"}
)

func init() { //nolint: gochecknoinits
	utilruntime.Must(registryv1.SchemeBuilder.AddToScheme(registryScheme))
}
