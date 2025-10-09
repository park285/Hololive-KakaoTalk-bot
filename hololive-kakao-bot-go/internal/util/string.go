package util

import "strings"

func TruncateString(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

func Normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// NormalizeSuffix removes common Korean suffixes (짱, 쨩) after normalization
func NormalizeSuffix(s string) string {
	normalized := Normalize(s)

	// Remove "짱" suffix
	if strings.HasSuffix(normalized, "짱") {
		return normalized[:len(normalized)-len("짱")]
	}

	// Remove "쨩" suffix
	if strings.HasSuffix(normalized, "쨩") {
		return normalized[:len(normalized)-len("쨩")]
	}

	return normalized
}

func NormalizeKey(name string) string {
	name = Normalize(name)
	if name == "" {
		return ""
	}

	var builder strings.Builder
	for _, r := range name {
		switch r {
		case ' ', '-', '_', '.', '!', '☆', '・', '\u2018', '\u2019', '\'', 'ー', '—':
			continue
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func Slugify(name string) string {
	name = Normalize(name)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "'", "")
	name = strings.ReplaceAll(name, ".", "")
	name = strings.ReplaceAll(name, "!", "")
	return name
}

func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
