// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	insightsv1alpha1 "github.com/openshift/api/insights/v1alpha1"
	labels "k8s.io/apimachinery/pkg/labels"
	listers "k8s.io/client-go/listers"
	cache "k8s.io/client-go/tools/cache"
)

// DataGatherLister helps list DataGathers.
// All objects returned here must be treated as read-only.
type DataGatherLister interface {
	// List lists all DataGathers in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*insightsv1alpha1.DataGather, err error)
	// Get retrieves the DataGather from the index for a given name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*insightsv1alpha1.DataGather, error)
	DataGatherListerExpansion
}

// dataGatherLister implements the DataGatherLister interface.
type dataGatherLister struct {
	listers.ResourceIndexer[*insightsv1alpha1.DataGather]
}

// NewDataGatherLister returns a new DataGatherLister.
func NewDataGatherLister(indexer cache.Indexer) DataGatherLister {
	return &dataGatherLister{listers.New[*insightsv1alpha1.DataGather](indexer, insightsv1alpha1.Resource("datagather"))}
}
