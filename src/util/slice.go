package util

// Insert into a slice (without wanting to harm oneself)
func insert[T any](slice []T, index int, value T) []T {
	slice = append(slice, value)
	copy(slice[index+1:], slice[index:])
	slice[index] = value
	return slice
}
