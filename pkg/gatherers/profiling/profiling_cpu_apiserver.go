package profiling

import (
	"context"

	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
)

func (g *Gatherer) GatherAPIServerCPUProfile(ctx context.Context) ([]record.Record, []error) {
	pp, err := g.GetProfiles(ctx, ProfileCPU, 10)
	if err != nil {
		klog.Error(err)
	}
	rec := record.Record{
		Name: "cpu_profile",
		Item: marshal.RawByte(pp),
	}
	return []record.Record{rec}, nil
}
