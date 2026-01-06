package brain

import "regexp"

// gapIDPattern matches [gap X] markers with optional trailing whitespace.
// Examples: "[gap 17] ", "[gap 123]", "[gap  42] "
var gapIDPattern = regexp.MustCompile(`\[gap\s+\d+\]\s*`)

// SanitizeComment removes internal markers from user-facing content.
// Currently strips [gap X] patterns that should not be visible to end users.
// Returns the cleaned content and the count of patterns stripped.
func SanitizeComment(content string) (string, int) {
	matches := gapIDPattern.FindAllStringIndex(content, -1)
	count := len(matches)
	if count == 0 {
		return content, 0
	}
	return gapIDPattern.ReplaceAllString(content, ""), count
}
