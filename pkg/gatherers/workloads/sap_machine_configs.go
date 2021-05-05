package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
)

func GatherSAPMachineConfig(g *Gatherer, c chan<- gatherResult) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{errors: []error{err}}
		return
	}

	records, errs := gatherSAPMachineConfig(g.ctx, gatherDynamicClient)
	c <- gatherResult{records: records, errors: errs}
}

func gatherSAPMachineConfig(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	gvrMC := schema.GroupVersionResource{Group: "machineconfiguration.openshift.io", Version: "v1", Resource: "machineconfigs"}
	// gvrMCP := schema.GroupVersionResource{Group: "machineconfiguration.openshift.io", Version: "v1", Resource: "machineconfigpools"}
	mcList, err := dynamicClient.Resource(gvrMC).List(ctx, metav1.ListOptions{})
	klog.Warningf("------------------>>>>>> %#v", err)
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	records := []record.Record{}
	for _, mc := range mcList.Items {
		shouldBeGathered := mc.GetName() == "75-worker-sap-data-intelligence"

		for _, ownerRef := range mc.GetOwnerReferences() {
			if ownerRef.Kind == "ContainerRuntimeConfig" && ownerRef.Name == "sdi-pids-limit" {
				shouldBeGathered = true
				break
			}
		}

		if shouldBeGathered {
			records = append(records, record.Record{
				Name: fmt.Sprintf("config/machineconfigs/%s", mc.GetName()),
				Item: record.JSONMarshaller{Object: mc.Object},
			})
		}
	}

	return records, nil
}
