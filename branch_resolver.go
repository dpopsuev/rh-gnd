package dsr

import "strings"

// ResolveBranch substitutes placeholders in a branch pattern using the
// provided attributes map. Placeholders use {key} syntax. If the pattern
// is empty or contains no placeholders, it is returned as-is.
//
// Example: ResolveBranch("release-{ocp_version}", {"ocp_version": "4.21"})
// returns "release-4.21".
func ResolveBranch(pattern string, attrs map[string]string) string {
	if pattern == "" {
		return ""
	}
	result := pattern
	for k, v := range attrs {
		result = strings.ReplaceAll(result, "{"+k+"}", v)
	}
	return result
}
