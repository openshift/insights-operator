package workloads_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/insights-operator/pkg/gather"
	"github.com/openshift/insights-operator/pkg/gather/workloads"
)

func Test_Gatherer_Basic(t *testing.T) {
	gatherer := workloads.New(nil)
	assert.Equal(t, "workloads", gatherer.GetName())
	gatheringFunctions := gatherer.GetGatheringFunctions()
	assert.Greater(t, len(gatheringFunctions), 0)

	assert.Implements(t, (*gather.Interface)(nil), gatherer)
	assert.Implements(t, (*gather.CustomPeriodGatherer)(nil), gatherer)
}
