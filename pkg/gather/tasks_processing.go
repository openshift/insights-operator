package gather

import (
	"context"
	"sync"
	"time"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/record"
)

// Task represents gathering task where name is the name of a function and F is the function itself
type Task struct {
	Name string
	F    gatherers.GatheringClosure
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
	resultsChan := make(chan GatheringFunctionResult)

	// run all the tasks in the background and close the channel when they are finished
	go func() {
		var wg sync.WaitGroup
		workerNum := 5 // TODO: Get this from config
		tasksChan := make(chan Task)

		// create workers
		for i := 0; i < workerNum; i++ {
			wg.Add(1)
			go worker(ctx, &wg, tasksChan, resultsChan)
		}

		// supply workers with tasks
		for _, task := range tasks {
			tasksChan <- task
		}
		close(tasksChan)

		// wait for finish
		wg.Wait()
		close(resultsChan)
	}()

	return resultsChan
}

func worker(ctx context.Context, wg *sync.WaitGroup, tasksChan <-chan Task, resultsChan chan<- GatheringFunctionResult) {
	defer wg.Done()
	for task := range tasksChan {
		handleTask(ctx, task, resultsChan)
	}
}

func handleTask(ctx context.Context, task Task, resultsChan chan<- GatheringFunctionResult) {
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
