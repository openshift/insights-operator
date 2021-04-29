package clusterconfig_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/insights-operator/pkg/gather"
	"github.com/openshift/insights-operator/pkg/gather/clusterconfig"
)

func Test_Gatherer_Basic(t *testing.T) {
	gatherer := clusterconfig.New(nil, nil, nil, nil)
	assert.Equal(t, "clusterconfig", gatherer.GetName())
	gatheringFunctions := gatherer.GetGatheringFunctions()
	assert.Greater(t, len(gatheringFunctions), 0)

	assert.Implements(t, (*gather.Interface)(nil), gatherer)

	var g interface{} = gatherer
	_, ok := g.(gather.CustomPeriodGatherer)
	assert.False(t, ok, "should NOT implement gather.CustomPeriodGatherer")
}
