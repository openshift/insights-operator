package utils

// MinInt returns minimal value of ints
func MinInt(values ...int) int {
	minVal := values[0]

	for _, val := range values {
		if val < minVal {
			minVal = val
		}
	}

	return minVal
}

// MaxInt returns maximum value of ints
func MaxInt(values ...int) int {
	maxVal := values[0]

	for _, val := range values {
		if val > maxVal {
			maxVal = val
		}
	}

	return maxVal
}
