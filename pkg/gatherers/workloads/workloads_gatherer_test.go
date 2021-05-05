package workloads_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/gatherers/workloads"
)

func Test_Gatherer_Basic(t *testing.T) {
	gatherer := workloads.New(nil)
	assert.Equal(t, "workloads", gatherer.GetName())
	gatheringFunctions := gatherer.GetGatheringFunctions()
	assert.Greater(t, len(gatheringFunctions), 0)

	assert.Implements(t, (*gatherers.Interface)(nil), gatherer)
	assert.Implements(t, (*gatherers.CustomPeriodGatherer)(nil), gatherer)
}
