package gather

import (
	"context"
	"sync"
	"time"

	"github.com/openshift/insights-operator/pkg/gather/common"
	"github.com/openshift/insights-operator/pkg/record"
)

// Task represents gathering task where name is the name of a function and F is the function itself
type Task struct {
	Name string
	F    common.GatheringClosure
}

// GatheringFunctionResult represents the result of a function including results, errors
// and other useful data like time it took to process
type GatheringFunctionResult struct {
	FunctionName string
	Records      []record.Record
	Errs         []error
	Panic        interface{}
	TimeElapsed  time.Duration
	IgnoreErrors bool
}

// HandleTasksConcurrently processes tasks concurrently and returns iterator like channel with the results
// current implementation runs N goroutines where N is the number of tasks
func HandleTasksConcurrently(ctx context.Context, tasks []Task) chan GatheringFunctionResult {
	// TODO: consider using tasks pool with limited number of tasks run at the same time
	// so that memory usage doesn't grow linearly with the number of added gatherers

	resultsChan := make(chan GatheringFunctionResult)

	// run all the tasks in the background and close the channel when they are finished
	go func() {
		var wg sync.WaitGroup

		for _, task := range tasks {
			wg.Add(1)
			go handleTask(ctx, task, &wg, resultsChan)
		}

		wg.Wait()

		close(resultsChan)
	}()

	return resultsChan
}

func handleTask(ctx context.Context, task Task, wg *sync.WaitGroup, resultsChan chan GatheringFunctionResult) {
	defer wg.Done()

	startTime := time.Now()
	var panicked interface{}
	var records []record.Record
	var errs []error

	// wrap to a function to catch a panic
	func() {
		defer func() {
			if err := recover(); err != nil {
				panicked = err
			}
		}()

		records, errs = task.F.Run(ctx)
	}()

	resultsChan <- GatheringFunctionResult{
		FunctionName: task.Name,
		Records:      records,
		Errs:         errs,
		Panic:        panicked,
		TimeElapsed:  time.Since(startTime),
		IgnoreErrors: task.F.CanFail,
	}
}
