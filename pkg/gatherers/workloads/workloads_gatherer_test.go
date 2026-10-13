package workloads_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/gatherers/workloads"
)

func Test_Gatherer_Basic(t *testing.T) {
	gatherer := workloads.New(nil, nil)

	assert.Equal(t, "workloads", gatherer.GetName())
	assert.Implements(t, (*gatherers.Interface)(nil), gatherer)
	assert.Implements(t, (*gatherers.CustomPeriodGatherer)(nil), gatherer)

	gatheringFunctions, err := gatherer.GetGatheringFunctions(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 2, len(gatheringFunctions))
	assert.Contains(t, gatheringFunctions, "workload_info")
	assert.Contains(t, gatheringFunctions, "helmchart_info")
	assert.NotNil(t, gatheringFunctions["workload_info"].Run)
	assert.NotNil(t, gatheringFunctions["helmchart_info"].Run)
}

func Test_Gatherer_ShouldBeProcessedNow(t *testing.T) {
	gatherer := workloads.New(nil, nil)

	assert.True(t, gatherer.ShouldBeProcessedNow())

	gatherer.UpdateLastProcessingTime()
	assert.False(t, gatherer.ShouldBeProcessedNow())
}
