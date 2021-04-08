package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_SortAndRemoveDuplicates(t *testing.T) {
	testIntSlice := func(original []int, expected []int) {
		SortAndRemoveDuplicates(&original, func(i, j int) bool {
			return original[i] < original[j]
		})
		assert.Equal(t, expected, original)
	}

	testIntSlice([]int{1, 9, 5, -3, 9, -3, -3, 5, 2, 11}, []int{-3, 1, 2, 5, 9, 11})
	testIntSlice([]int{1, 2, 3, 4, 5}, []int{1, 2, 3, 4, 5})
	testIntSlice([]int{5, 4, 3, 2, 1}, []int{1, 2, 3, 4, 5})
	testIntSlice([]int{1, 2, 2, 3, 3, 3}, []int{1, 2, 3})
	testIntSlice([]int{3, 3, 3, 2, 2, 1, 2, 2, 3, 3, 3}, []int{1, 2, 3})
	testIntSlice([]int{1}, []int{1})
	testIntSlice([]int{}, []int{})

	testStringSlice := func(original []string, expected []string) {
		SortAndRemoveDuplicates(&original, func(i, j int) bool {
			return original[i] < original[j]
		})
		assert.Equal(t, expected, original)
	}

	testStringSlice([]string{"abc", "acd", "ade"}, []string{"abc", "acd", "ade"})
	testStringSlice([]string{"acd", "ade", "abc"}, []string{"abc", "acd", "ade"})
}

func Test_TakeLastNItemsFromByteArray(t *testing.T) {
	assert.Equal(t, []byte{0, 0, 1, 2, 3, 4, 5}, TakeLastNItemsFromByteArray([]byte{1, 2, 3, 4, 5}, 7))
	assert.Equal(t, []byte{1, 2, 3, 4, 5}, TakeLastNItemsFromByteArray([]byte{1, 2, 3, 4, 5}, 5))
	assert.Equal(t, []byte{4, 5}, TakeLastNItemsFromByteArray([]byte{1, 2, 3, 4, 5}, 2))
	assert.Equal(t, []byte{}, TakeLastNItemsFromByteArray([]byte{1, 2, 3, 4, 5}, 0))
	assert.Equal(t, []byte{0, 0, 0, 0}, TakeLastNItemsFromByteArray([]byte{0, 0, 0}, 4))
}
