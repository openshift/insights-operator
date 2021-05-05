package gather

import (
	"context"
	"fmt"
	"testing"
	"time"

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
	for i, _ := range results {
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

// TODO: consider removing. The idea of the test is to check that execution is not sequential,
// but the problem is that it's not guaranteed that it will take some specific time.
// Maybe it should be an integration test
func Test_HandleTasksConcurrently_Sleep(t *testing.T) {
	const N = 100

	var tasks []Task

	for i := 0; i < N; i++ {
		tasks = append(tasks, Task{
			F: gatherers.GatheringClosure{
				Run: func(context.Context) ([]record.Record, []error) {
					time.Sleep(10 * time.Millisecond)
					return nil, nil
				},
				CanFail: false,
			},
		})
	}

	startTime := time.Now()

	results := handleTasksConcurrentlyGatherTasks(tasks)

	elapsedTime := time.Since(startTime)
	assert.Len(t, results, N)

	assert.Less(t, elapsedTime, 100*time.Millisecond)
	assert.Greater(t, elapsedTime, 10*time.Millisecond)
}

func Test_HandleTasksConcurrently_CannotFail_Error(t *testing.T) {
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

func Test_HandleTasksConcurrently_CannotFail_Panic(t *testing.T) {
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

// TODO: more tests?

func handleTasksConcurrentlyGatherTasks(tasks []Task) []GatheringFunctionResult {
	resultsChan := HandleTasksConcurrently(context.Background(), tasks)

	var results []GatheringFunctionResult
	for result := range resultsChan {
		results = append(results, result)
	}

	return results
}
