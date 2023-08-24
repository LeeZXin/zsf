package listutil

func Contains[T comparable](t T, arr []T) bool {
	for _, a := range arr {
		if a == t {
			return true
		}
	}
	return false
}
