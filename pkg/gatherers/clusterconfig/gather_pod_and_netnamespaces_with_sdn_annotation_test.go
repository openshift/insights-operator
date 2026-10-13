package clusterconfig

import (
	"context"
	"testing"

	networkv1 "github.com/openshift/api/network/v1"
	networkfake "github.com/openshift/client-go/network/clientset/versioned/fake"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8Testing "k8s.io/client-go/testing"
)

func TestGatherNumberOfPodsAndNetnamespacesWithSDN(t *testing.T) {
	tests := []struct {
		name           string
		pods           []*v1.Pod
		netNamespaces  []*networkv1.NetNamespace
		expctedRecords []record.Record
	}{
		{
			name: "no data found",
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pod",
					},
				},
			},
			netNamespaces: []*networkv1.NetNamespace{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-netnamespace",
					},
				},
			},
			expctedRecords: nil,
		},
		{
			name: "some data found",
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pod-1",
						Annotations: map[string]string{
							"another-annotation": "true",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pod-2",
						Annotations: map[string]string{
							assignMacVlanAnn: "true",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pod-3",
						Annotations: map[string]string{
							assignMacVlanAnn: "",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pod-4",
						Annotations: map[string]string{
							assignMacVlanAnn: "false",
						},
					},
				},
			},
			netNamespaces: []*networkv1.NetNamespace{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-netnamespace-1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-netnamespace-2",
						Annotations: map[string]string{
							multicastEnabledAnn: "true",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-netnamespace-3",
						Annotations: map[string]string{
							multicastEnabledAnn: "false",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-netnamespace-4",
						Annotations: map[string]string{
							"aother-annotation": "true",
						},
					},
				},
			},
			expctedRecords: []record.Record{
				{
					Name: "aggregated/pods_and_netnamespaces_with_sdn_annotations",
					Item: record.JSONMarshaller{
						Object: dataRecord{
							NumberOfPods:          3,
							NumberOfNetnamespaces: 1,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeCli := kubefake.NewSimpleClientset()
			err := addObjectsToClientSet(kubeCli, tt.pods)
			assert.NoError(t, err)
			networkCli := networkfake.NewSimpleClientset()
			err = addObjectsToClientSet(networkCli, tt.netNamespaces)
			assert.NoError(t, err)
			records, errs := gatherNumberOfPodsAndNetnamespacesWithSDN(context.Background(), networkCli.NetworkV1(), kubeCli)
			assert.Empty(t, errs)
			assert.Equal(t, tt.expctedRecords, records)
		})
	}
}

func addObjectsToClientSet[C []T, T runtime.Object](cli k8Testing.FakeClient, obj C) error {
	for i := range obj {
		o := obj[i]
		err := cli.Tracker().Add(o)
		if err != nil {
			return err
		}
	}
	return nil
}
