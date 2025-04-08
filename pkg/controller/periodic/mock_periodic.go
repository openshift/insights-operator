package periodic

import (
	"context"

	configv1alpha1 "github.com/openshift/api/config/v1alpha1"

	"github.com/openshift/library-go/pkg/controller/factory"
	"k8s.io/klog/v2"
)

type InsightsDataGatherObserverMock struct {
	mockedDataPolicy configv1alpha1.DataPolicy
	mockedGatherers  []configv1alpha1.DisabledGatherer
}

func NewInsightsDataGatherObserverMock(
	mockedDataPolicy configv1alpha1.DataPolicy,
	mockedGatherers []configv1alpha1.DisabledGatherer,
) *InsightsDataGatherObserverMock {
	return &InsightsDataGatherObserverMock{
		mockedDataPolicy: mockedDataPolicy,
		mockedGatherers:  mockedGatherers,
	}
}

func (i InsightsDataGatherObserverMock) Name() string {
	return "InsightsDataGatherObserverMock"
}

func (i InsightsDataGatherObserverMock) Run(_ context.Context, _ int) {
	klog.Info("Running InsightsDataGatherObserverMock")
}

func (i InsightsDataGatherObserverMock) Sync(_ context.Context, _ factory.SyncContext) error {
	klog.Info("Syncing InsightsDataGatherObserverMock")
	return nil
}

func (i InsightsDataGatherObserverMock) GatherConfig() *configv1alpha1.GatherConfig {
	return &configv1alpha1.GatherConfig{
		DataPolicy:        i.mockedDataPolicy,
		DisabledGatherers: i.mockedGatherers,
		StorageSpec:       nil,
	}
}

func (i InsightsDataGatherObserverMock) GatherDisabled() bool {
	return false
}
