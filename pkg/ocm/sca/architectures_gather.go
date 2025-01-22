package sca

import (
	"context"
	"runtime"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Mapping of kubernetes architecture labels to the format used by SCA API
var kubernetesArchMapping = map[string]string{
	"386":     "x86",
	"amd64":   "x86_64",
	"ppc":     "ppc",
	"ppc64":   "ppc64",
	"ppc64le": "ppc64le",
	"s390":    "s390",
	"s390x":   "s390x",
	"ia64":    "ia64",
	"arm64":   "aarch64",
}

func getArch(arch string) string {
	if translation, ok := kubernetesArchMapping[arch]; ok {
		return translation
	}

	// Default to the arch of a node where operator is running
	return kubernetesArchMapping[runtime.GOARCH]
}

// gatherArchitectures connects to K8S API to retrieve the list of
// nodes and create a set of the present architectures
func (c *Controller) gatherArchitectures(ctx context.Context) (map[string]struct{}, error) {
	nodes, err := c.coreClient.Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	architectures := make(map[string]struct{})
	for i := range nodes.Items {
		nodeArch := nodes.Items[i].Status.NodeInfo.Architecture
		architectures[getArch(nodeArch)] = struct{}{}
	}

	return architectures, nil
}
