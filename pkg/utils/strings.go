package utils

// Map applies each of functions to passed slice
func Map(it []string, fn func(string) string) []string {
	outSlice := []string{}
	for _, str := range it {
		outSlice = append(outSlice, fn(str))
	}
	return outSlice
}
