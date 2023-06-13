package status

import (
	"fmt"
	"strings"
	"time"

	"github.com/openshift/api/insights/v1alpha1"
	v1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/insights-operator/pkg/gather"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DataGatheredCondition = "DataGathered"
	// NoDataGathered is a reason when there is no data gathered - e.g the resource is not in a cluster
	NoDataGatheredReason = "NoData"
	// Error is a reason when there is some error and no data gathered
	GatherErrorReason = "GatherError"
	// Panic is a reason when there is some error and no data gathered
	GatherPanicReason = "GatherPanic"
	// GatheredOK is a reason when data is gathered as expected
	GatheredOKReason = "GatheredOK"
	// GatheredWithError is a reason when data is gathered partially or with another error message
	GatheredWithErrorReason = "GatheredWithError"
)

// CreateOperatorGathererStatus creates GathererStatus attribute for the "insightsoperator.operator.openshift.io"
// custom resource type.
func CreateOperatorGathererStatus(gfr *gather.GathererFunctionReport) v1.GathererStatus {
	gs := v1.GathererStatus{
		Name: gfr.FuncName,
		LastGatherDuration: metav1.Duration{
			// v.Duration is in milliseconds and we need nanoseconds
			Duration: time.Duration(gfr.Duration * 1000000),
		},
	}

	gs.Conditions = createGathererConditions(gfr)
	return gs
}

// CreateDataGatherGathererStatus creates GathererStatus attribute for the "datagather.insights.openshift.io"
// custom resource type.
func CreateDataGatherGathererStatus(gfr *gather.GathererFunctionReport) v1alpha1.GathererStatus {
	gs := v1alpha1.GathererStatus{
		Name: gfr.FuncName,
		LastGatherDuration: metav1.Duration{
			// v.Duration is in milliseconds and we need nanoseconds
			Duration: time.Duration(gfr.Duration * 1000000),
		},
	}

	gs.Conditions = createGathererConditions(gfr)
	return gs
}

// createGathererConditions creates GathererConditions based on gatherer result passed in as
// GathererFunctionReport.
func createGathererConditions(gfr *gather.GathererFunctionReport) []metav1.Condition {
	conditions := []metav1.Condition{}

	con := metav1.Condition{
		Type:               DataGatheredCondition,
		LastTransitionTime: metav1.Now(),
		Status:             metav1.ConditionFalse,
		Reason:             NoDataGatheredReason,
	}

	if gfr.Panic != nil {
		con.Reason = GatherPanicReason
		con.Message = gfr.Panic.(string)
	}

	if gfr.RecordsCount > 0 {
		con.Status = metav1.ConditionTrue
		con.Reason = GatheredOKReason
		con.Message = fmt.Sprintf("Created %d records in the archive.", gfr.RecordsCount)

		if len(gfr.Errors) > 0 {
			con.Reason = GatheredWithErrorReason
			con.Message = fmt.Sprintf("%s Error: %s", con.Message, strings.Join(gfr.Errors, ","))
		}

		conditions = append(conditions, con)
		return conditions
	}

	if len(gfr.Errors) > 0 {
		con.Reason = GatherErrorReason
		con.Message = strings.Join(gfr.Errors, ",")
	}

	conditions = append(conditions, con)
	return conditions
}

// DataGatherStatusToOperatorGatherStatus copies "DataGatherStatus" from "datagather.openshift.io" and creates
// "GatherStatus" for "insightsoperator.operator.openshift.io"
func DataGatherStatusToOperatorGatherStatus(dgGatherStatus *v1alpha1.DataGatherStatus) v1.GatherStatus {
	operatorGatherStatus := v1.GatherStatus{}
	operatorGatherStatus.LastGatherTime = dgGatherStatus.FinishTime
	operatorGatherStatus.LastGatherDuration = metav1.Duration{
		Duration: dgGatherStatus.FinishTime.Sub(dgGatherStatus.StartTime.Time),
	}

	for _, g := range dgGatherStatus.Gatherers {
		gs := v1.GathererStatus{
			Name:               g.Name,
			LastGatherDuration: g.LastGatherDuration,
			Conditions:         g.Conditions,
		}
		operatorGatherStatus.Gatherers = append(operatorGatherStatus.Gatherers, gs)
	}

	return operatorGatherStatus
}
