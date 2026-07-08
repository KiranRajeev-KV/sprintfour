package document

import "fmt"

func SubstringByRuneIndex(text string, start, end int) (string, error) {
	if start < 0 || end < 0 || end < start {
		return "", fmt.Errorf("invalid rune span [%d,%d)", start, end)
	}

	runes := []rune(text)
	if end > len(runes) {
		return "", fmt.Errorf("rune span [%d,%d) out of bounds for length %d", start, end, len(runes))
	}

	return string(runes[start:end]), nil
}
