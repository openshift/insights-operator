package anonymization

import (
	"context"
	"slices"

	insightsv1 "github.com/openshift/api/insights/v1"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/record"
)

type WorkloadAnonymizer struct {
	configurator configobserver.Interface
	dataPolicy   insightsv1.DataPolicyOption
}

func NewWorkloadAnonymizer(
	ctx context.Context,
	configurator configobserver.Interface,
) *WorkloadAnonymizer {
	return &WorkloadAnonymizer{
		configurator: configurator,
	}
}

func (wa *WorkloadAnonymizer) WithDataPolicies(dataPolicy ...insightsv1.DataPolicyOption) *WorkloadAnonymizer {
	if slices.Contains(dataPolicy, insightsv1.DataPolicyOptionObfuscateWorkloadNames) {
		wa.dataPolicy = insightsv1.DataPolicyOptionObfuscateWorkloadNames
	}
	return wa
}

func (wa *WorkloadAnonymizer) IsEnabled() bool {
	obfuscation := wa.configurator.Config().DataReporting.Obfuscation
	// support secret still has precedence
	if obfuscateWorkloadNames(obfuscation) {
		return true
	}

	if wa.dataPolicy != "" {
		return wa.dataPolicy == insightsv1.DataPolicyOptionObfuscateWorkloadNames
	}

	return false
}

// obfuscateWorkloadNames tells whether WorkloadNames should be "obfuscated" or not
func obfuscateWorkloadNames(o config.Obfuscation) bool {
	for _, ov := range o {
		if ov == config.WorkloadNames {
			return true
		}
	}
	return false
}

func (wa *WorkloadAnonymizer) Skip() bool {
	return true
}

// This function should implement WorkloadAnonymization logic, that is now directly in the
// gather_dvo_metrics gatherer.
// The issue is tracked here: https://redhat.atlassian.net/browse/CCXDEV-15394
func (wa *WorkloadAnonymizer) AnonymizeData(memoryRecord *record.MemoryRecord) (*record.MemoryRecord, error) {
	return nil, nil
}

func (wa *WorkloadAnonymizer) GetType() AnonymizerType {
	return WorkloadNamesAnonymizerType
}
