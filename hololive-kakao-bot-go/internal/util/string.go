package util

import "strings"

// TruncateString truncates a string to maxRunes characters (rune-based, not byte-based)
// If truncated, appends "..." to the result
func TruncateString(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

// Normalize performs basic string normalization (lowercase + trim)
func Normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// NormalizeKey normalizes a name for use as a lookup key by removing special characters
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

// Slugify converts a name to URL-friendly slug format
func Slugify(name string) string {
	name = Normalize(name)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "'", "")
	name = strings.ReplaceAll(name, ".", "")
	name = strings.ReplaceAll(name, "!", "")
	return name
}

// Contains checks if a string slice contains a specific item
func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
