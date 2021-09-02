package utils

import (
	"fmt"
	"sort"
	"strings"
)

// SumErrors simply sorts the errors and joins them with commas
func SumErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}

	var errStrings []string
	for _, err := range errs {
		errStrings = append(errStrings, err.Error())
	}

	sort.Strings(errStrings)
	errStrings = UniqueStrings(errStrings)

	return fmt.Errorf("%s", strings.Join(errStrings, ", "))
}

// ErrorsToStrings turns error slice to string slice
func ErrorsToStrings(errs []error) []string {
	var result []string
	for _, err := range errs {
		result = append(result, err.Error())
	}

	return result
}
