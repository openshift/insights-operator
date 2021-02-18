package clusterconfig

import (
	"time"

	registryv1 "github.com/openshift/api/imageregistry/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"k8s.io/apimachinery/pkg/runtime"
)

const (
	// maxEventTimeInterval represents the "only keep events that are maximum 1h old"
	// TODO: make this dynamic like the reporting window based on configured interval
	maxEventTimeInterval = 1 * time.Hour
)

var (
	registryScheme = runtime.NewScheme()
	// logTailLines sets maximum number of lines to fetch from pod logs
	logTailLines = int64(100)

	defaultNamespaces = []string{"default", "kube-system", "kube-public"}
)

func init() {
	utilruntime.Must(registryv1.AddToScheme(registryScheme))
}
