package bcel

import (
	"fmt"
	"regexp"
	"strings"
)

var dotFieldRegexp = regexp.MustCompile(`\.\w+`)

func isAlphaNumeric(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// PreprocessColumnExpressions replaces all column expressions with the appropriate map access
// Example input: ".role_name == 'Admin'" -> "cols['role_name'] == 'Admin'"
func preprocessColumnExpressions(expr string) string {
	result := expr
	offset := 0

	result = dotFieldRegexp.ReplaceAllStringFunc(result, func(s string) string {
		matchIndex := strings.Index(expr[offset:], s) + offset
		if matchIndex > 0 && isAlphaNumeric(expr[matchIndex-1]) {
			offset = matchIndex + len(s)
			return s
		}

		offset = matchIndex + len(s)
		field := strings.TrimPrefix(s, ".")
		return fmt.Sprintf("cols['%s']", field)
	})

	return result
}
