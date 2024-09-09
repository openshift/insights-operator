package workloads

import (
	"context"
	"time"

	"k8s.io/client-go/rest"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
)

var workloadsGathererPeriod = time.Hour * 12

type Gatherer struct {
	gatherKubeConfig        *rest.Config
	gatherProtoKubeConfig   *rest.Config
	lastProcessingTime      time.Time
	runtimeExtractorEnabled bool
}

func New(gatherKubeConfig, gatherProtoKubeConfig *rest.Config, runtimeExtractorEnabled bool) *Gatherer {
	return &Gatherer{
		gatherKubeConfig:        gatherKubeConfig,
		gatherProtoKubeConfig:   gatherProtoKubeConfig,
		lastProcessingTime:      time.Unix(0, 0),
		runtimeExtractorEnabled: runtimeExtractorEnabled,
	}
}

func (g *Gatherer) GetName() string {
	return "workloads"
}

func (g *Gatherer) GetGatheringFunctions(context.Context) (map[string]gatherers.GatheringClosure, error) {
	return map[string]gatherers.GatheringClosure{
		"workload_info": {
			Run: func(ctx context.Context) ([]record.Record, []error) {
				return g.GatherWorkloadInfo(ctx)
			},
		},
		"helmchart_info": {
			Run: func(ctx context.Context) ([]record.Record, []error) {
				return g.GatherHelmInfo(ctx)
			},
		},
	}, nil
}

func (g *Gatherer) ShouldBeProcessedNow() bool {
	return utils.ShouldBeProcessedNow(g.lastProcessingTime, workloadsGathererPeriod)
}

func (g *Gatherer) UpdateLastProcessingTime() {
	g.lastProcessingTime = time.Now()
}
