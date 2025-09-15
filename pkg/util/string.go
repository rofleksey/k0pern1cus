package util

func TrimSuffixToNRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}

	runes := []rune(s)
	if len(runes) <= n {
		return s
	}

	if n <= 3 { // Not enough space for ellipsis
		return string(runes[:n])
	}

	return string(runes[:n-3]) + "..."
}
