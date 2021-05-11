package workloads

import (
	"context"
	"time"

	"k8s.io/client-go/rest"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/record"
)

var workloadsGathererPeriod = time.Hour * 12

type Gatherer struct {
	gatherProtoKubeConfig *rest.Config
	lastProcessingTime    time.Time
}

func New(gatherProtoKubeConfig *rest.Config) *Gatherer {
	return &Gatherer{
		gatherProtoKubeConfig: gatherProtoKubeConfig,
		lastProcessingTime:    time.Unix(0, 0),
	}
}

func (g *Gatherer) GetName() string {
	return "workloads"
}

func (g *Gatherer) GetGatheringFunctions() map[string]gatherers.GatheringClosure {
	return map[string]gatherers.GatheringClosure{
		"workload_info": {
			Run: func(ctx context.Context) ([]record.Record, []error) {
				return g.GatherWorkloadInfo(ctx)
			},
			CanFail: true,
		},
	}
}

func (g *Gatherer) ShouldBeProcessedNow() bool {
	timeToProcess := g.lastProcessingTime.Add(workloadsGathererPeriod)
	return time.Now().Equal(timeToProcess) || time.Now().After(timeToProcess)
}

func (g *Gatherer) UpdateLastProcessingTime() {
	g.lastProcessingTime = time.Now()
}
