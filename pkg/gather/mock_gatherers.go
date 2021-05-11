package gather

import (
	"context"
	"fmt"
	"time"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/record"
)

// MockGatherer is a mock gatherer collecting some fake data
type MockGatherer struct {
	SomeField string
	CanFail   bool
}

func (*MockGatherer) GetName() string { return "mock_gatherer" }

func (g *MockGatherer) GetGatheringFunctions() map[string]gatherers.GatheringClosure {
	return map[string]gatherers.GatheringClosure{
		"name": {
			Run: func(ctx context.Context) ([]record.Record, []error) {
				return g.GatherName(ctx)
			},
			CanFail: false,
		},
		"some_field": {
			Run: func(ctx context.Context) ([]record.Record, []error) {
				return g.GatherSomeField(ctx)
			},
			CanFail: false,
		},
		"3_records": {
			Run: func(ctx context.Context) ([]record.Record, []error) {
				return g.Gather3Records(ctx)
			},
			CanFail: false,
		},
		"errors": {
			Run: func(ctx context.Context) ([]record.Record, []error) {
				return g.GatherErrors(ctx)
			},
			CanFail: g.CanFail,
		},
		"panic": {
			Run: func(ctx context.Context) ([]record.Record, []error) {
				return g.GatherPanic(ctx)
			},
			CanFail: g.CanFail,
		},
	}
}

func (g *MockGatherer) GatherName(context.Context) ([]record.Record, []error) {
	return []record.Record{
		{
			Name: "name",
			Item: record.JSONMarshaller{Object: g.GetName()},
		},
	}, nil
}

func (g *MockGatherer) GatherSomeField(context.Context) ([]record.Record, []error) {
	return []record.Record{
		{
			Name: "some_field",
			Item: record.JSONMarshaller{Object: g.SomeField},
		},
	}, nil
}

func (g *MockGatherer) Gather3Records(context.Context) ([]record.Record, []error) {
	return []record.Record{
		{
			Name: "record_1",
			Item: record.JSONMarshaller{Object: "data 1"},
		},
		{
			Name: "record_2",
			Item: record.JSONMarshaller{Object: "data 2"},
		},
		{
			Name: "record_3",
			Item: record.JSONMarshaller{Object: "data 3"},
		},
	}, nil
}

func (g *MockGatherer) GatherErrors(context.Context) ([]record.Record, []error) {
	return nil, []error{
		fmt.Errorf("error1"),
		fmt.Errorf("error2"),
		fmt.Errorf("error3"),
	}
}

func (g *MockGatherer) GatherPanic(context.Context) ([]record.Record, []error) {
	panic("test panic")
	return nil, nil
}

// MockCustomPeriodGatherer is a mock for a gatherer with custom period
type MockCustomPeriodGatherer struct {
	Period             time.Duration
	lastProcessingTime time.Time
}

func (*MockCustomPeriodGatherer) GetName() string { return "mock_custom_period_gatherer" }

func (g *MockCustomPeriodGatherer) GetGatheringFunctions() map[string]gatherers.GatheringClosure {
	return map[string]gatherers.GatheringClosure{
		"period": {
			Run: func(ctx context.Context) ([]record.Record, []error) {
				return g.GatherPeriod(ctx)
			},
			CanFail: false,
		},
	}
}

func (g *MockCustomPeriodGatherer) ShouldBeProcessedNow() bool {
	timeToProcess := g.lastProcessingTime.Add(g.Period)
	return time.Now().Equal(timeToProcess) || time.Now().After(timeToProcess)
}

func (g *MockCustomPeriodGatherer) UpdateLastProcessingTime() {
	g.lastProcessingTime = time.Now()
}

func (g *MockCustomPeriodGatherer) GatherPeriod(context.Context) ([]record.Record, []error) {
	return []record.Record{
		{
			Name: "period",
			Item: record.JSONMarshaller{Object: g.Period},
		},
	}, nil
}

// MockCustomPeriodGathererNoPeriod is a mock for a CustomPeriodGatherer which just returns shouldBeProcessed value
// and updates ShouldBeProcessedNowWasCalledNTimes and UpdateLastProcessingTimeWasCalledNTimes when appropriate
// methods were called
type MockCustomPeriodGathererNoPeriod struct {
	ShouldBeProcessed                       bool
	ShouldBeProcessedNowWasCalledNTimes     int
	UpdateLastProcessingTimeWasCalledNTimes int
}

func (*MockCustomPeriodGathererNoPeriod) GetName() string {
	return "mock_custom_period_gatherer_no_period"
}

func (g *MockCustomPeriodGathererNoPeriod) GetGatheringFunctions() map[string]gatherers.GatheringClosure {
	return map[string]gatherers.GatheringClosure{
		"should_be_processed": {
			Run: func(ctx context.Context) ([]record.Record, []error) {
				return g.GatherShouldBeProcessed(ctx)
			},
			CanFail: false,
		},
	}
}

func (g *MockCustomPeriodGathererNoPeriod) ShouldBeProcessedNow() bool {
	g.ShouldBeProcessedNowWasCalledNTimes++
	return g.ShouldBeProcessed
}

func (g *MockCustomPeriodGathererNoPeriod) UpdateLastProcessingTime() {
	g.UpdateLastProcessingTimeWasCalledNTimes++
}

func (g *MockCustomPeriodGathererNoPeriod) GatherShouldBeProcessed(context.Context) ([]record.Record, []error) {
	return []record.Record{
		{
			Name: "should_be_processed",
			Item: record.JSONMarshaller{Object: g.ShouldBeProcessed},
		},
	}, nil
}
