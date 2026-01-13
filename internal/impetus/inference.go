package impetus

import (
	"regexp"
	"strings"
)

// Pattern defines a regex pattern and its associated impetus label.
type Pattern struct {
	Regex *regexp.Regexp
	Label string
}

// patterns defines the inference rules in priority order.
var patterns = []Pattern{
	{regexp.MustCompile(`(?i)github\.com/`), "GitHub discovery"},
	{regexp.MustCompile(`(?i)(twitter\.com|x\.com)/`), "X discovery"},
	{regexp.MustCompile(`(?i)(youtube\.com|youtu\.be)/`), "YouTube discovery"},
	{regexp.MustCompile(`(?i)(^|\s)(from\s+)?coaching[:\s]`), "Coaching"},
	{regexp.MustCompile(`(?i)(^|\s)session[:\s]`), "Session"},
	{regexp.MustCompile(`(?i)^(bug|fix)[:\s]`), "Bug fix"},
	{regexp.MustCompile(`(?i)^(feature|implemented)[:\s]`), "Feature"},
	{regexp.MustCompile(`(?i)linkedin\.com/`), "LinkedIn discovery"},
	{regexp.MustCompile(`(?i)reddit\.com/`), "Reddit discovery"},
	{regexp.MustCompile(`(?i)https?://`), "Web discovery"},
}

// Infer returns the impetus label for the given content.
// Returns empty string if no pattern matches.
func Infer(content string) string {
	label, _ := InferWithConfidence(content)
	return label
}

// InferWithConfidence returns the impetus label and confidence score.
// Confidence is 1.0 for specific patterns, 0.5 for generic web URLs.
func InferWithConfidence(content string) (string, float64) {
	content = strings.TrimSpace(content)
	for i, p := range patterns {
		if p.Regex.MatchString(content) {
			// Lower confidence for generic web discovery
			if i == len(patterns)-1 {
				return p.Label, 0.5
			}
			return p.Label, 1.0
		}
	}
	return "", 0.0
}
