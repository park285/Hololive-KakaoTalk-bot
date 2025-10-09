package util

// Max returns the maximum of two integers
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Min returns the minimum of two integers
func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Unique returns a slice of unique integers from the input
func Unique(nums []int) []int {
	seen := make(map[int]struct{})
	result := make([]int, 0, len(nums))

	for _, n := range nums {
		if _, exists := seen[n]; !exists {
			seen[n] = struct{}{}
			result = append(result, n)
		}
	}

	return result
}
