package clusterconfig

import (
	"context"
	"reflect"
	"testing"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
)

const testErrorMessage = "This is a test error"

type testError struct{}

func (e *testError) Error() string {
	return testErrorMessage
}

func mockGatherFunction1(_ *Gatherer, c chan<- gatherResult) {
	c <- gatherResult{[]record.Record{{
		Name: "config/mock1",
		Item: marshal.Raw{Str: "mock1"},
	}}, nil}
}

func mockGatherFunction2(_ *Gatherer, c chan<- gatherResult) {
	c <- gatherResult{[]record.Record{{
		Name: "config/mock2",
		Item: marshal.Raw{Str: "mock2"},
	}}, nil}
}

func mockGatherFunctionError(_ *Gatherer, c chan<- gatherResult) {
	c <- gatherResult{nil, []error{&testError{}}}
}

type mockRecorder struct {
	Recorded []record.Record
}

func (mr *mockRecorder) Record(r record.Record) error {
	mr.Recorded = append(mr.Recorded, r)
	return nil
}

type mockFailingRecorder struct{}

func (mr *mockFailingRecorder) Record(_ record.Record) error {
	return &testError{}
}

func initTest() Gatherer {
	gatherFunctions = map[string]gathering{
		"mock1": important(mockGatherFunction1),
		"mock2": important(mockGatherFunction2),
		"error": important(mockGatherFunctionError),
	}
	return Gatherer{ctx: context.Background()}
}

func cleanUp(cases []reflect.SelectCase) {
	remaining := len(cases)
	for remaining > 0 {
		chosen, _, _ := reflect.Select(cases)
		cases[chosen].Chan = reflect.ValueOf(nil)
		remaining--
	}
}

func Test_Gather(t *testing.T) {
	gatherer := initTest()
	ctx := context.Background()
	recorder := mockRecorder{}
	gatherList := []string{gatherAll}

	err := gatherer.Gather(ctx, gatherList, &recorder)

	expectedError := testErrorMessage
	if err.Error() != expectedError {
		t.Fatalf("unexpected error returned: Expected %s but got %s", expectedError, err.Error())
	}
	expectedRecordAmount := 3 // 2 successful gather function + 1 io report
	actualRecordAmount := len(recorder.Recorded)
	if actualRecordAmount != expectedRecordAmount {
		t.Fatalf("unexpected record amount, Expected %d, but got %d", expectedRecordAmount, actualRecordAmount)
	}
}

func Test_Gather_FailingRecorder(t *testing.T) {
	gatherer := initTest()
	ctx := context.Background()
	recorder := mockFailingRecorder{}
	gatherList := []string{gatherAll}

	err := gatherer.Gather(ctx, gatherList, &recorder)

	expectedError := "This is a test error, unable to record config/mock1: " +
		"This is a test error, unable to record config/mock2: " +
		"This is a test error, unable to record io status reports: This is a test error"
	if err.Error() != expectedError {
		t.Fatalf("unexpected error returned: Expected %s but got %s", expectedError, err.Error())
	}
}

func Test_Gather_StartEmpty(t *testing.T) {
	var gatherList []string
	var errors []string
	g := initTest()
	cases, starts, err := g.startGathering(gatherList, &errors)
	if cases != nil || starts != nil || err != nil {
		t.Fatalf("unexpected return values, expected: nil, nil, nil, got: %p, %p, %s", cases, starts, err)
	}
}

func Test_Gather_StartGathering(t *testing.T) {
	var errors []string
	g := initTest()
	gatherList := fullGatherList()
	expected := len(gatherList)

	cases, starts, err := g.startGathering(gatherList, &errors)
	lStarts := len(starts)
	lCases := len(cases)
	cleanUp(cases)

	if lCases != expected || lStarts != expected || err != nil {
		t.Fatalf(`unexpected return values:
		Expected %d cases got %d
		Expected %d starts got %d
		Err should be nil got %s`, expected, lCases, expected, lStarts, err)
	}
}

func Test_Gather_FullGatherList(t *testing.T) {
	initTest()
	gatherList := fullGatherList()
	expected := 3
	if got := len(gatherList); got != expected {
		t.Fatalf("unexpected number of gather functions returned, expected: %d received: %d", expected, got)
	}
}

func Test_Gather_SumErrors(t *testing.T) {
	errors := []string{
		"Error1",
		"Error2",
		"Error1",
		"Error3",
	}
	expected := "Error1, Error2, Error3"
	if got := sumErrors(errors).Error(); expected != got {
		t.Fatalf("unexpected error sum returned, expected: %s received: %s", expected, got)
	}
}
