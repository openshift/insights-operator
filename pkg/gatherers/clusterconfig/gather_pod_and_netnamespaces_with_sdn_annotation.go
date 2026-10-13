package clusterconfig

import (
	"context"

	networkv1client "github.com/openshift/client-go/network/clientset/versioned/typed/network/v1"
	"github.com/openshift/insights-operator/pkg/record"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	assignMacVlanAnn    = "pod.network.openshift.io/assign-macvlan"
	multicastEnabledAnn = "netnamespace.network.openshift.io/multicast-enabled"
)

// GatherNumberOfPodsAndNetnamespacesWithSDNAnnotations Collects number of Pods with the annotation:
// `pod.network.openshift.io/assign-macvlan`
// and also collects number of Netnamespaces with the annotation:
// `netnamespace.network.openshift.io/multicast-enabled: "true"`
//
// ### Sample data
// - docs/insights-archive-sample/aggregated/pods_and_netnamespaces_with_sdn_annotations.json
//
// ### Location in archive
// - `aggregated/pods_and_netnamespaces_with_sdn_annotations.json`
//
// ### Config ID
// `clusterconfig/pods_and_netnamespaces_with_sdn_annotations`
//
// ### Released version
// - 4.17.0
//
// ### Backported versions
//
// ### Changes
// None
func (g *Gatherer) GatherNumberOfPodsAndNetnamespacesWithSDNAnnotations(ctx context.Context) ([]record.Record, []error) {
	networkClient, err := networkv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}
	kubeCli, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherNumberOfPodsAndNetnamespacesWithSDN(ctx, networkClient, kubeCli)
}

type dataRecord struct {
	NumberOfPods          int `json:"pods_with_assign-macvlan_annotation"`
	NumberOfNetnamespaces int `json:"netnamespaces_with_multicast-enabled_annotation"`
}

func gatherNumberOfPodsAndNetnamespacesWithSDN(ctx context.Context,
	networkCli networkv1client.NetworkV1Interface,
	kubeCli kubernetes.Interface) ([]record.Record, []error) {
	var errs []error

	numberOfPods, err := getNumberOfPodsWithAnnotation(ctx, assignMacVlanAnn, kubeCli)
	if err != nil {
		errs = append(errs, err)
	}
	numberOfNetnamespaces, err := getNumberOfNetnamespacesWithAnnotation(ctx, multicastEnabledAnn, networkCli)
	if err != nil {
		errs = append(errs, err)
	}

	if numberOfNetnamespaces == 0 && numberOfPods == 0 {
		return nil, nil
	}

	return []record.Record{
		{
			Name: "aggregated/pods_and_netnamespaces_with_sdn_annotations",
			Item: record.JSONMarshaller{Object: dataRecord{
				NumberOfPods:          numberOfPods,
				NumberOfNetnamespaces: numberOfNetnamespaces,
			}},
		},
	}, errs
}

// getNumberOfPodsWithAnnotation lists all the Pods in the cluster and counts the ones with provided annotation
func getNumberOfPodsWithAnnotation(ctx context.Context, annotation string, kubeCli kubernetes.Interface) (int, error) {
	var continueValue string
	var numberOfPods int
	for {
		pods, err := kubeCli.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
			Limit:    500,
			Continue: continueValue,
		})
		if err != nil {
			return 0, err
		}

		for i := range pods.Items {
			pod := pods.Items[i]
			if _, ok := pod.Annotations[annotation]; ok {
				numberOfPods++
			}
		}

		if pods.Continue == "" {
			break
		}
		continueValue = pods.Continue
	}
	return numberOfPods, nil
}

// getNumberOfNetnamespacesWithAnnotation lists all the Netnamespaces in the cluster
// and counts the ones with provided annotation
func getNumberOfNetnamespacesWithAnnotation(ctx context.Context,
	annotation string,
	networkCli networkv1client.NetworkV1Interface) (int, error) {
	var numberOfNamespaces int
	var continueValue string

	for {
		netNamespaces, err := networkCli.NetNamespaces().List(ctx, metav1.ListOptions{
			Limit:    500,
			Continue: continueValue,
		})
		if err != nil {
			return 0, err
		}

		for i := range netNamespaces.Items {
			netNamespace := netNamespaces.Items[i]
			if v, ok := netNamespace.Annotations[annotation]; ok {
				if v == "true" {
					numberOfNamespaces++
				}
			}
		}

		if netNamespaces.Continue == "" {
			break
		}
		continueValue = netNamespaces.Continue
	}

	return numberOfNamespaces, nil
}
