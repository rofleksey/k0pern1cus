package util

func GetPtrOrZero[T any](value *T) T {
	if value == nil {
		var zero T

		return zero
	}

	return *value
}

func PtrOrDefault[T any](value *T, defaultValue T) *T {
	if value == nil {
		return &defaultValue
	}

	return value
}

func GetPtrOrDefault[T any](value *T, defaultValue T) T {
	if value == nil {
		return defaultValue
	}

	return *value
}

func NonNilSlice[T any](value []T) []T {
	if value == nil {
		return []T{}
	}

	return value
}

func NonNilMap[K comparable, V any](value map[K]V) map[K]V {
	if value == nil {
		return map[K]V{}
	}

	return value
}

func ToPtr[T any](value T) *T {
	return &value
}

func FirstOrNil[T any](slice []T) *T {
	if len(slice) == 0 {
		return nil
	}

	return &slice[0]
}

func PtrInt32ToPtrInt64(ptr *int32) *int64 {
	if ptr == nil {
		return nil
	}

	result := int64(*ptr)
	return &result
}

func PtrInt32ToPtrInt(ptr *int32) *int {
	if ptr == nil {
		return nil
	}

	result := int(*ptr)
	return &result
}

func PtrInt64ToPtrInt(ptr *int64) *int {
	if ptr == nil {
		return nil
	}

	result := int(*ptr)
	return &result
}

func PtrIntToPtrInt32(ptr *int) *int32 {
	if ptr == nil {
		return nil
	}

	result := int32(*ptr) //nolint:gosec
	return &result
}

func PtrIntToPtrInt64(ptr *int) *int64 {
	if ptr == nil {
		return nil
	}

	result := int64(*ptr)
	return &result
}

func PtrEquals[T comparable](a, b *T) bool {
	if a == nil && b == nil {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	return *a == *b
}

func Ternary[T any](condition bool, a, b T) T {
	if condition {
		return a
	}

	return b
}
