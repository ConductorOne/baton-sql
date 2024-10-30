package bcel

import (
	"fmt"
	"regexp"
	"strings"
)

var dotFieldRegexp = regexp.MustCompile(`\.\w+`)
var bareStringRegexp = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func isAlphaNumeric(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// preprocessExpressions replaces all column expressions with the appropriate map access.
// It also detects 'bare strings' and automatically quotes them.
// Example input: ".role_name == 'Admin'" -> "cols['role_name'] == 'Admin'".
func preprocessExpressions(expr string) string {
	if bareStringRegexp.MatchString(expr) {
		if expr == "true" || expr == "false" {
			return expr
		}

		return fmt.Sprintf(`"%s"`, expr)
	}

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
