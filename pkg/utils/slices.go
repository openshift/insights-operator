package utils

import (
	"reflect"
	"sort"
)

// SortAndRemoveDuplicates sorts the slice pointed by the provided pointer given the provided
// less function and removes repeated elements.
// The function panics if the provided interface is not a pointer to a slice.
func SortAndRemoveDuplicates(slicePtr interface{}, less func(i, j int) bool) {
	v := reflect.ValueOf(slicePtr).Elem()
	if v.Len() <= 1 {
		return
	}
	sort.Slice(v.Interface(), less)

	i := 0
	for j := 1; j < v.Len(); j++ {
		if !less(i, j) {
			continue
		}
		i++
		v.Index(i).Set(v.Index(j))
	}
	i++
	v.SetLen(i)
}


// TakeLastNItemsFromByteArray takes last N items from provided byte array
// or adds zeros to the beginning if there not enough space
func TakeLastNItemsFromByteArray(array []byte, desiredLength int) []byte {
	if len(array) > desiredLength {
		array = array[len(array)-desiredLength:]
	} else {
		for len(array) < desiredLength {
			array = append([]byte{0}, array...)
		}
	}

	return array
}
