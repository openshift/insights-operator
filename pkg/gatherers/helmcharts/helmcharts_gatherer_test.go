package helmcharts

import (
	"context"
	"testing"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/stretchr/testify/assert"
)

func Test_Gatherer_Basic(t *testing.T) {
	gatherer := New(nil, nil)
	assert.Equal(t, "helmcharts", gatherer.GetName())
	gatheringFunctions, err := gatherer.GetGatheringFunctions(context.TODO())
	assert.NoError(t, err)
	assert.Greater(t, len(gatheringFunctions), 0)

	assert.Implements(t, (*gatherers.Interface)(nil), gatherer)
	assert.Implements(t, (*gatherers.CustomPeriodGatherer)(nil), gatherer)
}
