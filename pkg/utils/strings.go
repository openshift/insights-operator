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

// UniqueStrings returns a new string slice where each element exists maximum once,
// the order of items is preserved (e.g. [9, 4, 9, 8, 1, 2, 2, 4, 3] becomes [9, 4, 8, 1, 2, 3])
func UniqueStrings(list []string) []string {
	if len(list) < 2 {
		return list
	}

	keys := make(map[string]bool)
	var set []string

	for _, entry := range list {
		if _, found := keys[entry]; !found {
			keys[entry] = true
			set = append(set, entry)
		}
	}

	return set
}
