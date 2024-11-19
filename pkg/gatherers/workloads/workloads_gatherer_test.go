package workloads_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/gatherers/workloads"
)

func Test_Gatherer_Basic(t *testing.T) {
	gatherer := workloads.New(nil, nil, false)
	assert.Equal(t, "workloads", gatherer.GetName())
	gatheringFunctions, err := gatherer.GetGatheringFunctions(context.Background())
	assert.NoError(t, err)
	assert.Greater(t, len(gatheringFunctions), 0)

	assert.Implements(t, (*gatherers.Interface)(nil), gatherer)
	assert.Implements(t, (*gatherers.CustomPeriodGatherer)(nil), gatherer)
}
