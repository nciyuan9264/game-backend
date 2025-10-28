package utils

func SafeSlice(slice []string, max int) []string {
	if len(slice) < max {
		return slice
	}
	return slice[:max]
}
