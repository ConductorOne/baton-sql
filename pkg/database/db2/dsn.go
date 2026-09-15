package db2

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// urlSchemeRegex matches a DSN that begins with a URL scheme (e.g. "db2://"); anchoring
// to the start keeps a native DSN whose value contains "://" (e.g. PWD=my://secret) from
// being misread as a URL.
var urlSchemeRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://`)

// ErrAmbiguousDSN is returned when a brace-quoted value in a native DB2 DSN appears to have
// swallowed a later, unrelated KEYWORD=value field. Rather than guess which interpretation
// is correct, the DSN is rejected: credential-adjacent parsing should fail loud, not silently
// drop a field.
var ErrAmbiguousDSN = errors.New("ambiguous DB2 DSN: a brace-quoted value appears to contain a later field")

// ParseNativeDSN reports whether dsn is DB2's native ODBC keyword=value form (not a URL),
// returning its DATABASE value if present. HOSTNAME, not DATABASE alone, is the native
// marker, since other engines' ODBC/ADO strings also carry DATABASE; every caller shares
// this one detector to avoid drift.
func ParseNativeDSN(dsn string) (string, bool, error) {
	if urlSchemeRegex.MatchString(dsn) {
		return "", false, nil
	}
	parts, err := splitDB2DSN(dsn)
	if err != nil {
		return "", false, err
	}
	var database string
	native, haveDB := false, false
	for _, part := range parts {
		keyword, value, found := strings.Cut(part, "=")
		if !found {
			continue
		}
		keyword = strings.TrimSpace(keyword)
		switch {
		case strings.EqualFold(keyword, "HOSTNAME"):
			native = true
		case strings.EqualFold(keyword, "DATABASE"):
			if !haveDB { // first DATABASE= wins
				value = strings.TrimSpace(value)
				if strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}") {
					value = value[1 : len(value)-1]
				}
				database, haveDB = value, true
			}
		}
	}
	return database, native, nil
}

// IsNativeDSN reports whether dsn is DB2's native ODBC keyword=value form. An ambiguous DSN
// (see ErrAmbiguousDSN) is treated as not native, so callers fail loudly downstream instead
// of routing a DSN whose fields could not be reliably split.
func IsNativeDSN(dsn string) bool {
	_, native, err := ParseNativeDSN(dsn)
	return err == nil && native
}

// DSNDatabase returns the DATABASE keyword value from a native DB2 DSN, or "" if absent or
// if the DSN is ambiguous (see ErrAmbiguousDSN).
func DSNDatabase(dsn string) string {
	database, _, _ := ParseNativeDSN(dsn)
	return database
}

// matchBraces pairs each '{' with the '}' that actually closes it via LIFO stack
// matching, so an earlier unterminated '{' can't steal a later value's closing '}'.
// Unmatched braces have no entry in the returned map.
func matchBraces(s string) map[int]int {
	pairs := make(map[int]int)
	var stack []int
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			stack = append(stack, i)
		case '}':
			if n := len(stack); n > 0 {
				open := stack[n-1]
				stack = stack[:n-1]
				pairs[open] = i
			}
		}
	}
	return pairs
}

// bareFieldPattern matches ";KEYWORD=" inside a brace-quoted span: a sign that the span has
// swallowed a separate KEYWORD=value field rather than deliberately quoting one value.
var bareFieldPattern = regexp.MustCompile(`;\s*[A-Za-z][A-Za-z0-9 _]*=`)

// splitDB2DSN splits a native DB2 DSN on ';', treating '{' as ODBC quoting only when it
// opens a value and has a genuine matching '}' (per matchBraces). If that quoted span itself
// looks like it swallowed a later KEYWORD=value field (per bareFieldPattern), the DSN is
// rejected with ErrAmbiguousDSN instead of silently dropping that field.
func splitDB2DSN(dsn string) ([]string, error) {
	pairs := matchBraces(dsn)
	var parts []string
	start := 0
	braceEnd := -1        // index of the '}' that closes the current quoted value, or -1
	atValueStart := false // at a value position (right after '=', across whitespace) outside braces
	for i := 0; i < len(dsn); i++ {
		if braceEnd != -1 {
			if i == braceEnd {
				braceEnd = -1
				atValueStart = false
			}
			continue
		}
		switch dsn[i] {
		case '{':
			if atValueStart {
				if end, ok := pairs[i]; ok {
					if bareFieldPattern.MatchString(dsn[i+1 : end]) {
						return nil, fmt.Errorf("%w: %q", ErrAmbiguousDSN, dsn[i:end+1])
					}
					braceEnd = end
				}
			}
			atValueStart = false
		case '=':
			atValueStart = true
		case ';':
			parts = append(parts, dsn[start:i])
			start = i + 1
			atValueStart = false
		case ' ', '\t':
			// keep atValueStart so "DATABASE= {my;db}" still brace-detects.
		default:
			atValueStart = false
		}
	}
	return append(parts, dsn[start:]), nil
}

// Keywords derived from the URL itself; query parameters may not override them.
// Anyone needing full control over these can pass a native DB2 DSN instead.
var reservedDSNKeywords = map[string]bool{
	"HOSTNAME": true,
	"DATABASE": true,
	"PORT":     true,
	"PROTOCOL": true,
	"UID":      true,
	"PWD":      true,
}

// quoteDB2Value returns v in a form safe to embed in DB2's semicolon-delimited
// keyword=value DSN format, brace-quoting per ODBC convention when needed.
// A literal '}' cannot be represented inside a braced value, so it is rejected.
func quoteDB2Value(v string) (string, error) {
	if strings.Contains(v, "}") {
		return "", fmt.Errorf("value must not contain '}'")
	}
	if strings.ContainsAny(v, ";{= ") {
		return "{" + v + "}", nil
	}
	return v, nil
}

// convertToDB2DSN converts URL format to DB2 DSN format.
func convertToDB2DSN(dsn string) (string, error) {
	// If it's already in DB2's native keyword=value format, return as-is.
	// URL-format DSNs are exempt so those markers may appear in credentials.
	_, native, err := ParseNativeDSN(dsn)
	if err != nil {
		return "", fmt.Errorf("invalid native DB2 DSN: %w", err)
	}
	if native {
		return dsn, nil
	}

	parsedURL, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("invalid DSN format: %w", err)
	}

	if parsedURL.Scheme != "db2" {
		return "", fmt.Errorf("expected db2:// scheme, got %s", parsedURL.Scheme)
	}

	hostname, err := quoteDB2Value(parsedURL.Hostname())
	if err != nil {
		return "", fmt.Errorf("invalid hostname: %w", err)
	}
	port := parsedURL.Port()
	if port == "" {
		port = "50000" // Default DB2 port
	}

	database := strings.TrimPrefix(parsedURL.Path, "/")
	if database == "" {
		return "", fmt.Errorf("database name is required in DSN path")
	}
	quotedDatabase, err := quoteDB2Value(database)
	if err != nil {
		return "", fmt.Errorf("invalid database name: %w", err)
	}

	var username, password string
	if parsedURL.User != nil {
		username = parsedURL.User.Username()
		password, _ = parsedURL.User.Password()
	}

	// Build DB2 DSN format
	// HOSTNAME=SERVER_NAME;PORT=DB_PORT;DATABASE=DATABASE_NAME;UID=USER_ID;PWD=PASSWORD
	var dsnParts []string
	dsnParts = append(dsnParts, fmt.Sprintf("HOSTNAME=%s", hostname))
	dsnParts = append(dsnParts, fmt.Sprintf("DATABASE=%s", quotedDatabase))
	dsnParts = append(dsnParts, fmt.Sprintf("PORT=%s", port))
	dsnParts = append(dsnParts, "PROTOCOL=TCPIP")

	if username != "" {
		quoted, err := quoteDB2Value(username)
		if err != nil {
			return "", fmt.Errorf("invalid username: %w", err)
		}
		dsnParts = append(dsnParts, fmt.Sprintf("UID=%s", quoted))
	}
	if password != "" {
		quoted, err := quoteDB2Value(password)
		if err != nil {
			return "", fmt.Errorf("invalid password: %w", err)
		}
		dsnParts = append(dsnParts, fmt.Sprintf("PWD=%s", quoted))
	}

	// Forward URL query parameters as additional DB2 DSN keywords (e.g.
	// ?SECURITY=SSL), sorted for deterministic output.
	params := parsedURL.Query()
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		keyword := strings.ToUpper(key)
		if strings.ContainsAny(keyword, ";={}") {
			return "", fmt.Errorf("invalid connection parameter name %q", key)
		}
		if reservedDSNKeywords[keyword] {
			return "", fmt.Errorf("connection parameter %q conflicts with a reserved DSN keyword; use the native DB2 DSN format instead", key)
		}
		values := params[key]
		if len(values) != 1 || seen[keyword] {
			return "", fmt.Errorf("connection parameter %q specified multiple times", key)
		}
		seen[keyword] = true
		quoted, err := quoteDB2Value(values[0])
		if err != nil {
			return "", fmt.Errorf("invalid value for connection parameter %q: %w", key, err)
		}
		dsnParts = append(dsnParts, fmt.Sprintf("%s=%s", keyword, quoted))
	}

	return strings.Join(dsnParts, ";"), nil
}
