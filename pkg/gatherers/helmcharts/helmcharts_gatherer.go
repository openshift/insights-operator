package helmcharts

import (
	"context"
	"time"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
	"k8s.io/client-go/rest"
)

var helmChartsGathererPeriod = time.Hour * 24

type Gatherer struct {
	gatherKubeConfig      *rest.Config
	gatherProtoKubeConfig *rest.Config
	lastProcessingTime    time.Time
}

func New(gatherKubeConfig, gatherProtoKubeConfig *rest.Config) *Gatherer {
	return &Gatherer{
		gatherKubeConfig:      gatherKubeConfig,
		gatherProtoKubeConfig: gatherProtoKubeConfig,
		lastProcessingTime:    time.Unix(0, 0),
	}
}

func (g *Gatherer) GetName() string {
	return "helmcharts"
}

func (g *Gatherer) GetGatheringFunctions(context.Context) (map[string]gatherers.GatheringClosure, error) {
	return map[string]gatherers.GatheringClosure{
		"helm_info": {
			Run: func(ctx context.Context) ([]record.Record, []error) {
				return g.GatherHelmInfo(ctx)
			},
		},
	}, nil
}

func (g *Gatherer) ShouldBeProcessedNow() bool {
	return utils.ShouldBeProcessedNow(g.lastProcessingTime, helmChartsGathererPeriod)
}

func (g *Gatherer) UpdateLastProcessingTime() {
	g.lastProcessingTime = time.Now()
}
