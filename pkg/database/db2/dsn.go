package db2

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Keywords derived from the URL itself; query parameters may not override them.
// Anyone needing full control over these can pass a native DB2 DSN instead.
// PROTOCOL is intentionally NOT reserved: it defaults to TCPIP but may be
// overridden via a ?PROTOCOL= query parameter (needed by some DB2-for-i paths).
var reservedDSNKeywords = map[string]bool{
	"HOSTNAME": true,
	"DATABASE": true,
	"PORT":     true,
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
	// If it's already in DB2 format (contains HOSTNAME= or DATABASE=), return as-is.
	// URL-format DSNs are exempt from this check so those markers may appear in credentials.
	if !strings.HasPrefix(dsn, "db2://") && !strings.HasPrefix(dsn, "db2i://") && (strings.Contains(dsn, "HOSTNAME=") || strings.Contains(dsn, "DATABASE=")) {
		return dsn, nil
	}

	parsedURL, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("invalid DSN format: %w", err)
	}

	// "db2" targets DB2 LUW; "db2i" targets DB2 for i (IBM i / AS-400), which
	// differs only in its default DRDA port. All other handling is identical.
	if parsedURL.Scheme != "db2" && parsedURL.Scheme != "db2i" {
		return "", fmt.Errorf("expected db2:// or db2i:// scheme, got %s", parsedURL.Scheme)
	}
	isIBMi := parsedURL.Scheme == "db2i"

	hostname, err := quoteDB2Value(parsedURL.Hostname())
	if err != nil {
		return "", fmt.Errorf("invalid hostname: %w", err)
	}
	port := parsedURL.Port()
	if port == "" {
		// DB2 LUW defaults to 50000; DB2 for i (DRDA) defaults to 446.
		if isIBMi {
			port = "446"
		} else {
			port = "50000"
		}
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
	// PROTOCOL defaults to TCPIP but is overridable via a ?PROTOCOL= query param
	// (appended, uppercased, by the query-parameter loop below). Only set the
	// default here when the caller did not supply one.
	hasProtocol := false
	for k := range parsedURL.Query() {
		if strings.EqualFold(k, "PROTOCOL") {
			hasProtocol = true
			break
		}
	}
	if !hasProtocol {
		dsnParts = append(dsnParts, "PROTOCOL=TCPIP")
	}

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
