package profiling

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/record"
)

const (
	ProfileCPU      = "debug/pprof/profile"
	ProfileTypeHeap = "debug/pprof/heap"
)

var profilingGathererPeriod = time.Hour * 12

type Gatherer struct {
	gatherKubeConfig   *rest.Config
	lastProcessingTime time.Time
}

func New(gatherKubeConfig *rest.Config) *Gatherer {
	kubeConfig := rest.CopyConfig(gatherKubeConfig)
	// TODO: Get a token that has the necessary permissions, for testing I used the token in my ~/.kube/config
	kubeConfig.BearerToken = "fancy-token"
	kubeConfig.NegotiatedSerializer = scheme.Codecs
	kubeConfig.GroupVersion = &schema.GroupVersion{}
	kubeConfig.APIPath = "/"
	return &Gatherer{
		gatherKubeConfig:   kubeConfig,
		lastProcessingTime: time.Unix(0, 0),
	}
}

func (g *Gatherer) GetName() string {
	return "profiling"
}

func (g *Gatherer) GetGatheringFunctions() map[string]gatherers.GatheringClosure {
	return map[string]gatherers.GatheringClosure{
		"apiserver_cpu_profiling": {
			Run: func(ctx context.Context) ([]record.Record, []error) {
				return g.GatherAPIServerCPUProfile(ctx)
			},
			CanFail: true,
		},
	}
}

func (g *Gatherer) ShouldBeProcessedNow() bool {
	timeToProcess := g.lastProcessingTime.Add(profilingGathererPeriod)
	return time.Now().Equal(timeToProcess) || time.Now().After(timeToProcess)
}

func (g *Gatherer) UpdateLastProcessingTime() {
	g.lastProcessingTime = time.Now()
}

// GET
// /api/debug/pprof/profile?seconds=10
func (g *Gatherer) GetProfiles(ctx context.Context, profile string, seconds int) ([]byte, error) {
	profilingRESTClient, err := rest.RESTClientFor(g.gatherKubeConfig)
	if err != nil {
		klog.Warningf("Unable to load profiling client, profiling will not be collected: %v", err)
		return nil, nil
	}
	data, err := profilingRESTClient.Get().AbsPath(profile).
		Param("seconds", fmt.Sprint(seconds)).
		DoRaw(ctx)
	if err != nil {
		klog.Errorf("Unable to retrieve profiling data: %v", err)
		return nil, err
	}
	return data, nil
}
