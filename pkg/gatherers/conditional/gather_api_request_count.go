package conditional

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
)

// APIRequestCount defines a type used when marshaling into JSON
type APIRequestCount struct {
	ResourceName        string `json:"resource"`
	RemovedInRelease    string `json:"removed_in_release"`
	TotalRequestCount   int64  `json:"total_request_count"`
	LastDayRequestCount int64  `json:"last_day_request_count"`
}

// BuildGatherAPIRequestCounts Collects API requests counts for the resources mentioned in
// the alert provided as a string parameter.
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/conditional/alerts/APIRemovedInNextEUSReleaseInUse/api_request_counts.json
//
// ### Location in archive
// - `conditional/alerts/{alert_name}/api_request_counts.json`
//
// ### Config ID
// `conditional/api_request_counts_of_resource_from_alert`
//
// ### Released version
// - 4.10.0
//
// ### Backported versions
// - 4.9.6+
//
// ### Changes
// None
func (g *Gatherer) BuildGatherAPIRequestCounts(paramsInterface interface{}) (gatherers.GatheringClosure, error) {
	params, ok := paramsInterface.(GatherAPIRequestCountsParams)
	if !ok {
		return gatherers.GatheringClosure{}, fmt.Errorf(
			"unexpected type in paramsInterface, expected %T, got %T",
			GatherAPIRequestCountsParams{}, paramsInterface,
		)
	}

	return gatherers.GatheringClosure{
		Run: func(ctx context.Context) ([]record.Record, []error) {
			dynamicClient, err := dynamic.NewForConfig(g.gatherProtoKubeConfig)
			if err != nil {
				return nil, []error{err}
			}
			records, errs := g.gatherAPIRequestCounts(ctx, dynamicClient, params.AlertName)
			if errs != nil {
				return records, errs
			}
			return records, nil
		},
	}, nil
}

func (g *Gatherer) gatherAPIRequestCounts(ctx context.Context,
	dynamicClient dynamic.Interface, alertName string) ([]record.Record, []error) {
	resources := make(map[string]struct{})
	for _, labels := range g.firingAlerts[alertName] {
		resourceName := fmt.Sprintf("%s.%s.%s", labels["resource"], labels["version"], labels["group"])
		resources[resourceName] = struct{}{}
	}

	gvr := schema.GroupVersionResource{Group: "apiserver.openshift.io", Version: "v1", Resource: "apirequestcounts"}
	apiReqCountsList, err := dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	var records []record.Record
	var errrs []error
	var apiReqCounts []APIRequestCount
	for i := range apiReqCountsList.Items {
		it := apiReqCountsList.Items[i]

		// filter only resources we're interested in
		if _, ok := resources[it.GetName()]; ok {
			totalReqCount, err := utils.NestedInt64Wrapper(it.Object, "status", "requestCount")
			if err != nil {
				errrs = append(errrs, err)
			}
			lastDayReqCount, err := utils.NestedInt64Wrapper(it.Object, "status", "currentHour", "requestCount")
			if err != nil {
				errrs = append(errrs, err)
			}
			removedInRel, err := utils.NestedStringWrapper(it.Object, "status", "removedInRelease")
			if err != nil {
				errrs = append(errrs, err)
			}
			apiReqCount := APIRequestCount{
				TotalRequestCount:   totalReqCount,
				LastDayRequestCount: lastDayReqCount,
				ResourceName:        it.GetName(),
				RemovedInRelease:    removedInRel,
			}
			apiReqCounts = append(apiReqCounts, apiReqCount)
		}
	}
	records = append(records, record.Record{
		Name: fmt.Sprintf("%v/alerts/%s/api_request_counts", g.GetName(), alertName),
		Item: record.JSONMarshaller{Object: apiReqCounts},
	})
	return records, errrs
}
