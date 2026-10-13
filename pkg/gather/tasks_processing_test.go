package gather

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/record"
)

func Test_HandleTasksConcurrently(t *testing.T) {
	const N = 3

	var tasks []Task

	for i := 0; i < N; i++ {
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
		},
	}})

	assert.Len(t, results, 1)
	results[0].TimeElapsed = 0
	assert.Equal(t, results, []GatheringFunctionResult{
		{
			Panic: "test panic",
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
		},
	}})

	assert.Len(t, results, 1)
	results[0].TimeElapsed = 0
	assert.Equal(t, results, []GatheringFunctionResult{
		{
			Errs: []error{
				fmt.Errorf("test error"),
			},
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
		},
	}})

	assert.Len(t, results, 1)
	results[0].TimeElapsed = 0
	assert.Equal(t, results, []GatheringFunctionResult{
		{
			Errs: []error{
				fmt.Errorf("test error"),
			},
		},
	})
}

func Test_HandleTasksConcurrently_CanFail_Panic(t *testing.T) {
	results := handleTasksConcurrentlyGatherTasks([]Task{{
		F: gatherers.GatheringClosure{
			Run: func(context.Context) ([]record.Record, []error) {
				panic("test panic")
			},
		},
	}})

	assert.Len(t, results, 1)
	results[0].TimeElapsed = 0
	assert.Equal(t, results, []GatheringFunctionResult{
		{
			Panic: "test panic",
		},
	})
}

func handleTasksConcurrentlyGatherTasks(tasks []Task) []GatheringFunctionResult {
	resultsChan := HandleTasksConcurrently(context.Background(), tasks)

	var results []GatheringFunctionResult
	for result := range resultsChan {
		results = append(results, result)
	}

	return results
}

func Test_worker(_ *testing.T) {
	var wg sync.WaitGroup
	tasksChan := make(chan Task)
	resultsChan := make(chan GatheringFunctionResult)

	wg.Add(1)
	go worker(context.TODO(), 0, &wg, tasksChan, resultsChan)

	tasksChan <- Task{
		Name: "",
		F:    gatherers.GatheringClosure{},
	}
	close(tasksChan)

	<-resultsChan
}

func Test_handleTask(t *testing.T) {
	resultsChan := make(chan GatheringFunctionResult)
	go handleTask(context.TODO(), Task{
		Name: "",
		F:    gatherers.GatheringClosure{}, // Run is nil so it produces an error
	}, resultsChan)

	result := <-resultsChan
	assert.EqualError(t, result.Panic.(error), "runtime error: invalid memory address or nil pointer dereference")
}
