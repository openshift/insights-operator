package clusterconfig_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/gatherers/clusterconfig"
)

func Test_Gatherer_Basic(t *testing.T) {
	gatherer := clusterconfig.New(nil, nil, nil, nil)
	assert.Equal(t, "clusterconfig", gatherer.GetName())
	gatheringFunctions := gatherer.GetGatheringFunctions()
	assert.Greater(t, len(gatheringFunctions), 0)

	assert.Implements(t, (*gatherers.Interface)(nil), gatherer)

	var g interface{} = gatherer
	_, ok := g.(gatherers.CustomPeriodGatherer)
	assert.False(t, ok, "should NOT implement gather.CustomPeriodGatherer")
}
