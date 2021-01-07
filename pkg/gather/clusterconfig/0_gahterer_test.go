package clusterconfig

import (
	"reflect"
	"testing"
)

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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := uniqueStrings(tt.arr); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("uniqueStrings() = %v, want %v", got, tt.want)
			}
		})
	}
}
