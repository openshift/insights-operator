package gather

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/record"
)

func Test_HandleTasksConcurrently(t *testing.T) {
	const N = 3

	var tasks []Task

	for i := 0; i < N; i++ {
		i := i
		tasks = append(tasks, Task{
			Name: fmt.Sprintf("task_%v", i),
			F: gatherers.GatheringClosure{
				Run: func(context.Context) ([]record.Record, []error) {
					var records []record.Record

					for j := 0; j < 2; j++ {
						records = append(records, record.Record{
							Name: fmt.Sprintf("task_%v_record_%v", i, j),
						})
					}

					return records, nil
				},
				CanFail: false,
			},
		})
	}

	results := handleTasksConcurrentlyGatherTasks(tasks)
	for i := range results {
		results[i].TimeElapsed = 0
	}

	assert.Len(t, results, 3)
	assert.ElementsMatch(t, results, []GatheringFunctionResult{
		{
			FunctionName: "task_0",
			Records: []record.Record{
				{Name: "task_0_record_0"},
				{Name: "task_0_record_1"},
			},
		},
		{
			FunctionName: "task_1",
			Records: []record.Record{
				{Name: "task_1_record_0"},
				{Name: "task_1_record_1"},
			},
		},
		{
			FunctionName: "task_2",
			Records: []record.Record{
				{Name: "task_2_record_0"},
				{Name: "task_2_record_1"},
			},
		},
	})
}

func Test_HandleTasksConcurrently_CannotFail_Panic(t *testing.T) {
	results := handleTasksConcurrentlyGatherTasks([]Task{{
		F: gatherers.GatheringClosure{
			Run: func(context.Context) ([]record.Record, []error) {
				panic("test panic")
			},
			CanFail: false,
		},
	}})

	assert.Len(t, results, 1)
	results[0].TimeElapsed = 0
	assert.Equal(t, results, []GatheringFunctionResult{
		{
			Panic:        "test panic",
			IgnoreErrors: false,
		},
	})
}

func Test_HandleTasksConcurrently_CannotFail_Error(t *testing.T) {
	results := handleTasksConcurrentlyGatherTasks([]Task{{
		F: gatherers.GatheringClosure{
			Run: func(context.Context) ([]record.Record, []error) {
				return nil, []error{
					fmt.Errorf("test error"),
				}
			},
			CanFail: false,
		},
	}})

	assert.Len(t, results, 1)
	results[0].TimeElapsed = 0
	assert.Equal(t, results, []GatheringFunctionResult{
		{
			Errs: []error{
				fmt.Errorf("test error"),
			},
			IgnoreErrors: false,
		},
	})
}

func Test_HandleTasksConcurrently_CanFail_Error(t *testing.T) {
	results := handleTasksConcurrentlyGatherTasks([]Task{{
		F: gatherers.GatheringClosure{
			Run: func(context.Context) ([]record.Record, []error) {
				return nil, []error{
					fmt.Errorf("test error"),
				}
			},
			CanFail: true,
		},
	}})

	assert.Len(t, results, 1)
	results[0].TimeElapsed = 0
	assert.Equal(t, results, []GatheringFunctionResult{
		{
			Errs: []error{
				fmt.Errorf("test error"),
			},
			IgnoreErrors: true,
		},
	})
}

func Test_HandleTasksConcurrently_CanFail_Panic(t *testing.T) {
	results := handleTasksConcurrentlyGatherTasks([]Task{{
		F: gatherers.GatheringClosure{
			Run: func(context.Context) ([]record.Record, []error) {
				panic("test panic")
			},
			CanFail: true,
		},
	}})

	assert.Len(t, results, 1)
	results[0].TimeElapsed = 0
	assert.Equal(t, results, []GatheringFunctionResult{
		{
			Panic:        "test panic",
			IgnoreErrors: true,
		},
	})
}

// TODO: write a test testing that handleTasksConcurrently is actually concurrent.
// We can employ some threads synchronization magic which would cause execution to timeout when run sequentially
// and work just fine when run in parallel.
// TODO: more tests?

func handleTasksConcurrentlyGatherTasks(tasks []Task) []GatheringFunctionResult {
	resultsChan := HandleTasksConcurrently(context.Background(), tasks)

	var results []GatheringFunctionResult
	for result := range resultsChan {
		results = append(results, result)
	}

	return results
}
