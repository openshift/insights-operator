package utils

// StringInSlice simply checks if the string is present in slice
func StringInSlice(str string, slice []string) bool {
	for _, item := range slice {
		if item == str {
			return true
		}
	}

	return false
}
