package db2

import (
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

// ParseNativeDSN reports whether dsn is DB2's native ODBC keyword=value form (not a URL),
// returning its DATABASE value if present. HOSTNAME, not DATABASE alone, is the native
// marker, since other engines' ODBC/ADO strings also carry DATABASE; every caller shares
// this one detector to avoid drift.
func ParseNativeDSN(dsn string) (string, bool) {
	if urlSchemeRegex.MatchString(dsn) {
		return "", false
	}
	var database string
	native, haveDB := false, false
	for _, part := range splitDB2DSN(dsn) {
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
	return database, native
}

// IsNativeDSN reports whether dsn is DB2's native ODBC keyword=value form.
func IsNativeDSN(dsn string) bool {
	_, native := ParseNativeDSN(dsn)
	return native
}

// DSNDatabase returns the DATABASE keyword value from a native DB2 DSN, or "" if absent.
func DSNDatabase(dsn string) string {
	database, _ := ParseNativeDSN(dsn)
	return database
}

// splitDB2DSN splits a native DB2 DSN on ';', treating '{' as ODBC quoting only when it
// opens a value and is later closed by '}'; an unterminated or misplaced '{' is literal,
// so HOSTNAME/DATABASE markers stay visible instead of being silently swallowed.
func splitDB2DSN(dsn string) []string {
	var parts []string
	start := 0
	braced := false       // inside a {...} quoted value
	atValueStart := false // at a value position (right after '=', across whitespace) outside braces
	for i := 0; i < len(dsn); i++ {
		switch dsn[i] {
		case '}':
			braced = false
			atValueStart = false
		case '{':
			if atValueStart && strings.IndexByte(dsn[i:], '}') != -1 {
				braced = true
			}
			atValueStart = false
		case '=':
			if !braced {
				atValueStart = true
			}
		case ';':
			if !braced {
				parts = append(parts, dsn[start:i])
				start = i + 1
			}
			atValueStart = false
		case ' ', '\t':
			// keep atValueStart so "DATABASE= {my;db}" still brace-detects.
		default:
			atValueStart = false
		}
	}
	return append(parts, dsn[start:])
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
	if IsNativeDSN(dsn) {
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
