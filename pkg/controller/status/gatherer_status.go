package status

import (
	"fmt"
	"math"
	"strings"
	"time"

	insightsv1 "github.com/openshift/api/insights/v1"
	v1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/insights-operator/pkg/gather"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DataGatheredCondition = "DataGathered"
	// NoDataGatheredReason is a reason when there is no data gathered - e.g the resource is not in a cluster
	NoDataGatheredReason = "NoData"
	// GatherErrorReason is a reason when there is some error and no data gathered
	GatherErrorReason = "GatherError"
	// GatherPanicReason is a reason when there is some error and no data gathered
	GatherPanicReason = "GatherPanic"
	// GatheredOKReason is a reason when data is gathered as expected
	GatheredOKReason = "GatheredOK"
	// GatheredWithErrorReason is a reason when data is gathered partially or with another error message
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
func CreateDataGatherGathererStatus(report *gather.GathererFunctionReport) (*insightsv1.GathererStatus, error) {
	seconds, err := durationMillisToSeconds(report.Duration)
	if err != nil {
		return nil, err
	}

	return &insightsv1.GathererStatus{
		Name:              report.FuncName,
		LastGatherSeconds: &seconds,
		Conditions:        createGathererConditions(report),
	}, nil
}

// durationMillisToSeconds safely converts milliseconds to seconds as int32.
func durationMillisToSeconds(ms int64) (int32, error) {
	seconds := ms / 1000
	if seconds > math.MaxInt32 || seconds < math.MinInt32 {
		return 0, fmt.Errorf("duration %dms overflows int32", ms)
	}
	return int32(seconds), nil
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
		con.Message = fmt.Sprintf("%s", gfr.Panic)
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

// DataGatherStatusToOperatorStatus copies "DataGatherStatus" from "datagather.openshift.io" and creates
// "Status" for "insightsoperator.operator.openshift.io"
func DataGatherStatusToOperatorStatus(dg *insightsv1.DataGather) v1.InsightsOperatorStatus {
	operatorStatus := v1.InsightsOperatorStatus{}
	operatorStatus.GatherStatus = v1.GatherStatus{
		LastGatherTime: dg.Status.FinishTime,
		LastGatherDuration: metav1.Duration{
			Duration: dg.Status.FinishTime.Sub(dg.Status.StartTime.Time),
		},
	}

	operatorStatus.InsightsReport = v1.InsightsReport{
		DownloadedAt: dg.Status.InsightsReport.DownloadedTime,
	}

	for _, g := range dg.Status.Gatherers {
		lastGatherSeconds := int32(0)
		if g.LastGatherSeconds != nil {
			lastGatherSeconds = *g.LastGatherSeconds
		}

		gs := v1.GathererStatus{
			Name: g.Name,
			LastGatherDuration: metav1.Duration{
				Duration: time.Duration(lastGatherSeconds) * time.Second,
			},
			Conditions: g.Conditions,
		}
		operatorStatus.GatherStatus.Gatherers = append(operatorStatus.GatherStatus.Gatherers, gs)
	}

	for _, hc := range dg.Status.InsightsReport.HealthChecks {
		operatorHch := v1.HealthCheck{
			Description: hc.Description,
			TotalRisk:   totalRiskToInt32(hc.TotalRisk),
			State:       v1.HealthCheckEnabled,
			AdvisorURI:  hc.AdvisorURI,
		}
		operatorStatus.InsightsReport.HealthChecks = append(operatorStatus.InsightsReport.HealthChecks, operatorHch)
	}
	return operatorStatus
}

func totalRiskToInt32(totalRisk insightsv1.TotalRisk) int32 {
	switch totalRisk {
	case insightsv1.TotalRiskLow:
		return 1
	case insightsv1.TotalRiskModerate:
		return 2
	case insightsv1.TotalRiskImportant:
		return 3
	case insightsv1.TotalRiskCritical:
		return 4
	default:
		return 0
	}
}

func Int32ToTotalRisk(totalRisk int32) insightsv1.TotalRisk {
	switch totalRisk {
	case 1:
		return insightsv1.TotalRiskLow
	case 2:
		return insightsv1.TotalRiskModerate
	case 3:
		return insightsv1.TotalRiskImportant
	case 4:
		return insightsv1.TotalRiskCritical
	default:
		return insightsv1.TotalRiskLow
	}
}
