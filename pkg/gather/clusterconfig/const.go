package clusterconfig

import (
	"time"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	registryv1 "github.com/openshift/api/imageregistry/v1"
	networkv1 "github.com/openshift/api/network/v1"

	"k8s.io/apimachinery/pkg/runtime"
)

const (
	maxNamespacesLimit = 1000
	// maxEventTimeInterval represents the "only keep events that are maximum 1h old"
	// TODO: make this dynamic like the reporting window based on configured interval
	maxEventTimeInterval = 1 * time.Hour
)

var (
	registryScheme = runtime.NewScheme()
	networkScheme = runtime.NewScheme()
	// logTailLines sets maximum number of lines to fetch from pod logs
	logTailLines = int64(100)

	defaultNamespaces = []string{"default", "kube-system", "kube-public"}
)
func init() {
	utilruntime.Must(registryv1.AddToScheme(registryScheme))
	utilruntime.Must(networkv1.AddToScheme(networkScheme))
}