package clusterconfig

import (
	"context"
	"reflect"
	"testing"

	"github.com/openshift/insights-operator/pkg/record"
	"k8s.io/apimachinery/pkg/api/errors"
)

func mockGatherFunction1(g *Gatherer, c chan<- gatherResult) {
	c <- gatherResult{[]record.Record{{
		Name: "config/mock1",
		Item: Raw{"mock1"},
	}}, nil}
}

func mockGatherFunction2(g *Gatherer, c chan<- gatherResult) {
	c <- gatherResult{[]record.Record{{
		Name: "config/mock2",
		Item: Raw{"mock2"},
	}}, nil}
}

func mockGatherFunctionError(g *Gatherer, c chan<- gatherResult) {
	c <- gatherResult{nil, []error{&errors.StatusError{}}}
}

func init_test() Gatherer {
	gatherFunctions = map[string]gatherFunction{
		"mock1": mockGatherFunction1,
		"mock2": mockGatherFunction2,
		"error": mockGatherFunctionError,
	}
	return Gatherer{ctx: context.Background()}
}

func clean_up(cases []reflect.SelectCase) {
	remaining := len(cases)
	for remaining > 0 {
		chosen, _, _ := reflect.Select(cases)
		cases[chosen].Chan = reflect.ValueOf(nil)
		remaining -= 1
	}
}

func Test_startGathering_empty(t *testing.T) {
	var gatherList []string
	var errors []string
	g := init_test()
	cases, starts, err := g.startGathering(gatherList, &errors)
	if cases != nil || starts != nil || err != nil {
		t.Fatalf("unexpected return values, expected: nil, nil, nil, got: %p, %p, %s", cases, starts, err)
	}

}

func Test_startGathering(t *testing.T) {
	var errors []string
	g := init_test()
	gatherList := fullGatherList()
	expected := len(gatherList)

	cases, starts, err := g.startGathering(gatherList, &errors)
	l_starts := len(starts)
	l_cases := len(cases)
	clean_up(cases)

	if l_cases != expected || l_starts != expected || err != nil {
		t.Fatalf("unexpected return values: \nExpected %d cases got %d \nExpected %d starts got %d \n Err should be nil got %s", expected, l_cases, expected, l_starts, err)
	}

}

func Test_fullGatherList(t *testing.T) {
	init_test()
	gatherList := fullGatherList()
	expected := 3
	if got := len(gatherList); got != expected {
		t.Fatalf("unexpected number of gather functions returned, expected: %d received: %d", expected, got)
	}
}

func Test_sumErrors(t *testing.T) {
	errors := []string {
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

func Test_uniqueStrings(t *testing.T) {
	tests := []struct {
		name string
		arr  []string
		want []string
	}{
		{arr: nil, want: nil},
		{arr: []string{}, want: []string{}},
		{arr: []string{"a", "a", "a"}, want: []string{"a"}},
		{arr: []string{"a", "b", "b"}, want: []string{"a", "b"}},
		{arr: []string{"a", "a", "b"}, want: []string{"a", "b"}},
		{arr: []string{"a", "b"}, want: []string{"a", "b"}},
		{arr: []string{"a"}, want: []string{"a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := uniqueStrings(tt.arr); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("uniqueStrings() = %v, want %v", got, tt.want)
			}
		})
	}
}
