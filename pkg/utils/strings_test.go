package utils

import (
	"reflect"
	"testing"
)

func Test_UniqueStrings(t *testing.T) {
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
		{
			arr:  []string{"9", "4", "9", "8", "1", "2", "2", "4", "3"},
			want: []string{"9", "4", "8", "1", "2", "3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := UniqueStrings(tt.arr); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("uniqueStrings() = %v, want %v", got, tt.want)
			}
		})
	}
}
