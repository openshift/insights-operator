package sca

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
		architectures[nodeArch] = struct{}{}
	}
	return architectures, nil
}
