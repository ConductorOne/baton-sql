package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/conductorone/baton-sql/pkg/database/db2"
	"github.com/conductorone/baton-sql/pkg/database/hdb"
	"github.com/conductorone/baton-sql/pkg/database/mysql"
	"github.com/conductorone/baton-sql/pkg/database/oracle"
	"github.com/conductorone/baton-sql/pkg/database/postgres"
	"github.com/conductorone/baton-sql/pkg/database/sqlserver"
	"github.com/conductorone/baton-sql/pkg/database/vertica"
)

var DSNREnvRegex = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

type DbEngine uint8

const (
	Unknown DbEngine = iota
	MySQL
	PostgreSQL
	SQLite
	MSSQL
	Oracle
	HDB
	Vertica
	DB2
)

// ConnectOptions represents the structured configuration used to build a DSN.
// Any field may include ${ENV_VAR} placeholders that will be expanded before use.
type ConnectOptions struct {
	DSN string

	Scheme   string
	Host     string
	Port     string
	Database string

	User     string
	Password string

	Params map[string]string
}

func updateFromEnv(dsn string) (string, error) {
	var err error

	result := DSNREnvRegex.ReplaceAllStringFunc(dsn, func(match string) string {
		varName := match[2 : len(match)-1]

		value, exists := os.LookupEnv(varName)
		if !exists {
			err = errors.Join(err, fmt.Errorf("environment variable %s is not set", varName))
			return match
		}
		return value
	})
	if err != nil {
		return "", err
	}

	return result, nil
}

// extractPlaceholders replaces ${...} placeholders with unique sentinels
// using the _PH-999000_ format for all placeholders to avoid collisions.
// Port sentinels are expanded before URL parsing (in expandDSN) since ports must be numeric.
// Returns: the string with sentinels, a mapping of sentinel->value, and any error.
func extractPlaceholders(s string) (string, map[string]string, error) {
	mapping := make(map[string]string)
	counter := 0
	var err error

	result := DSNREnvRegex.ReplaceAllStringFunc(s, func(match string) string {
		sentinel := fmt.Sprintf("_PH-999%03d_", counter)
		varName := match[2 : len(match)-1] // Extract VAR from ${VAR}

		// Look up the environment variable immediately
		value, exists := os.LookupEnv(varName)
		if !exists {
			err = errors.Join(err, fmt.Errorf("environment variable %s is not set", varName))
			return sentinel // Return sentinel anyway to allow URL parsing for better error messages
		}

		mapping[sentinel] = value
		counter++
		return sentinel
	})

	if err != nil {
		return "", nil, err
	}

	return result, mapping, nil
}

// expandWithMapping expands sentinels in a string by replacing them with their
// corresponding values from the mapping. The sentinel format (_PH-999000_) includes
// a prefix and suffix to avoid collisions with literal patterns in the DSN.
func expandWithMapping(s string, mapping map[string]string) string {
	result := s
	for sentinel, value := range mapping {
		result = strings.ReplaceAll(result, sentinel, value)
	}
	return result
}

// expandUserInfo expands environment variable placeholders in the URL's user info component.
// It handles the special case where an entire "user:password" string might be in a single variable.
// The url.UserPassword function automatically handles URL encoding of special characters.
func expandUserInfo(parsedUrl *url.URL, mapping map[string]string) {
	if parsedUrl.User == nil {
		return
	}

	username := parsedUrl.User.Username()
	password, hasPass := parsedUrl.User.Password()

	// Expand sentinels in username
	expandedUser := expandWithMapping(username, mapping)

	// Expand sentinels in password
	var expandedPass string
	if hasPass {
		expandedPass = expandWithMapping(password, mapping)
	}

	// Handle the case where the entire userinfo (user:password) is in a single variable.
	// For example: ${CREDENTIALS} where CREDENTIALS="admin:p@ss#word"
	if strings.Contains(expandedUser, ":") && !hasPass {
		parts := strings.SplitN(expandedUser, ":", 2)
		expandedUser = parts[0]
		expandedPass = parts[1]
		hasPass = true
	}

	// url.UserPassword automatically handles URL encoding of special characters
	if hasPass {
		parsedUrl.User = url.UserPassword(expandedUser, expandedPass)
	} else {
		parsedUrl.User = url.User(expandedUser)
	}
}

// expandHost expands environment variable placeholders in the URL's host component.
// It also handles the case where the host variable contains a path component.
func expandHost(parsedUrl *url.URL, mapping map[string]string) {
	if parsedUrl.Host != "" {
		expandedHost := expandWithMapping(parsedUrl.Host, mapping)

		// Check if the expanded host contains a path (e.g., "localhost:3306/dbname")
		// If so, split it into host:port and path
		if strings.Contains(expandedHost, "/") {
			parts := strings.SplitN(expandedHost, "/", 2)
			parsedUrl.Host = parts[0]
			// Prepend the path part to the existing path
			if parts[1] != "" {
				if parsedUrl.Path == "" {
					parsedUrl.Path = "/" + parts[1]
				} else {
					parsedUrl.Path = "/" + parts[1] + parsedUrl.Path
				}
			}
		} else {
			parsedUrl.Host = expandedHost
		}
	}
}

// expandPath expands environment variable placeholders in the URL's path component.
func expandPath(parsedUrl *url.URL, mapping map[string]string) {
	if parsedUrl.Path != "" {
		parsedUrl.Path = expandWithMapping(parsedUrl.Path, mapping)
	}
}

// expandQuery expands environment variable placeholders in the URL's query string.
func expandQuery(parsedUrl *url.URL, mapping map[string]string) {
	if parsedUrl.RawQuery != "" {
		values, err := url.ParseQuery(parsedUrl.RawQuery)
		if err != nil {
			// Fallback: if parsing fails for some reason, do a direct expansion.
			// This preserves prior behavior rather than failing entirely.
			parsedUrl.RawQuery = expandWithMapping(parsedUrl.RawQuery, mapping)
			return
		}

		newValues := url.Values{}
		for key, vals := range values {
			expandedKey := expandWithMapping(key, mapping)
			for _, v := range vals {
				expandedVal := expandWithMapping(v, mapping)
				newValues.Add(expandedKey, expandedVal)
			}
		}
		parsedUrl.RawQuery = newValues.Encode()
	}
}

// expandFragment expands environment variable placeholders in the URL's fragment.
func expandFragment(parsedUrl *url.URL, mapping map[string]string) {
	if parsedUrl.Fragment != "" {
		parsedUrl.Fragment = expandWithMapping(parsedUrl.Fragment, mapping)
	}
}

// expandPortSentinel expands the port sentinel in the DSN before URL parsing.
// Ports must be numeric for URL parsing, so we detect and expand port sentinels
// (which use _PH-999000_ format) before passing to url.Parse.
// The mapping is modified in place to remove the port sentinel.
func expandPortSentinel(sentinelDSN string, mapping map[string]string) string {
	// Pattern: host:_PH-999XXX_/ or host:_PH-999XXX_? or host:_PH-999XXX_
	if !strings.Contains(sentinelDSN, "://") {
		return sentinelDSN
	}

	schemeEnd := strings.Index(sentinelDSN, "://")
	afterScheme := sentinelDSN[schemeEnd+3:]

	// Find the authority part (userinfo@host:port or host:port)
	authEnd := strings.IndexAny(afterScheme, "/?#")
	if authEnd == -1 {
		authEnd = len(afterScheme)
	}
	authority := afterScheme[:authEnd]

	// Find the last colon in authority (this should be the port separator)
	atIndex := strings.LastIndex(authority, "@")
	colonIndex := strings.LastIndex(authority, ":")

	// If there's a colon after @ (or no @), check if what follows is a sentinel
	if colonIndex > atIndex {
		portPart := authority[colonIndex+1:]
		// Check if portPart matches our sentinel pattern
		if strings.HasPrefix(portPart, "_PH-") && strings.HasSuffix(portPart, "_") {
			if value, ok := mapping[portPart]; ok {
				// Replace port sentinel with its value before parsing
				beforePort := sentinelDSN[:schemeEnd+3+colonIndex+1]
				afterPort := sentinelDSN[schemeEnd+3+len(authority):]
				sentinelDSN = beforePort + value + afterPort
				// Remove from mapping so it doesn't get expanded again
				delete(mapping, portPart)
			}
		}
	}

	return sentinelDSN
}

// expandDSN expands environment variable placeholders in a DSN using a three-phase approach:
// 1. Replace ${...} with safe sentinel values and lookup env vars
// 2. Parse the URL structure with sentinels
// 3. Expand sentinels component-by-component with appropriate encoding
//
// This approach ensures that special characters in environment variables (like #, @, :)
// don't break URL parsing, since they are expanded after the URL structure is established.
func expandDSN(dsn string) (string, error) {
	// Phase 1: Replace ${...} with sentinels and lookup env vars
	sentinelDSN, mapping, err := extractPlaceholders(dsn)
	if err != nil {
		return "", err
	}

	// If there are no placeholders, return as-is
	if len(mapping) == 0 {
		return dsn, nil
	}

	// Special case: if the entire DSN is a single variable (e.g., "${DSN}"),
	// just return the expanded value directly without parsing
	matches := DSNREnvRegex.FindAllString(dsn, -1)
	if len(matches) == 1 && strings.TrimSpace(dsn) == strings.TrimSpace(matches[0]) {
		// Get the variable name
		varName := matches[0][2 : len(matches[0])-1]
		value, exists := os.LookupEnv(varName)
		if !exists {
			return "", fmt.Errorf("environment variable %s is not set", varName)
		}
		return value, nil
	}

	// Special case: if the scheme is a variable, we need to expand it before parsing
	// Check if the DSN starts with a sentinel followed by ://
	if strings.Contains(sentinelDSN, "://") {
		schemeEnd := strings.Index(sentinelDSN, "://")
		if schemeEnd > 0 {
			schemeSentinel := sentinelDSN[:schemeEnd]
			// Check if this is a sentinel we created
			if strings.HasPrefix(schemeSentinel, "_PH-") {
				if value, ok := mapping[schemeSentinel]; ok {
					// Replace the scheme sentinel with the actual scheme
					sentinelDSN = value + sentinelDSN[schemeEnd:]
				}
			}
		}
	}

	// Special case: Expand port sentinel before parsing (ports must be numeric for URL parsing)
	sentinelDSN = expandPortSentinel(sentinelDSN, mapping)

	// Phase 2: Parse with sentinels
	parsedUrl, err := url.Parse(sentinelDSN)
	if err != nil {
		return "", fmt.Errorf("invalid DSN structure: %w", err)
	}

	// Phase 3: Expand by component with appropriate encoding
	expandUserInfo(parsedUrl, mapping)
	expandHost(parsedUrl, mapping)
	expandPath(parsedUrl, mapping)
	expandQuery(parsedUrl, mapping)
	expandFragment(parsedUrl, mapping)

	return parsedUrl.String(), nil
}

// ResolveDatabaseName returns the database opts would connect to, merging the DSN
// path with the structured field. Empty when neither supplies one.
func ResolveDatabaseName(opts ConnectOptions) string {
	if opts.Database != "" {
		if expanded, err := expandValue(opts.Database); err == nil && expanded != "" {
			return expanded
		}
	}
	parsedUrl, err := buildConnectionURL(opts)
	if err != nil || parsedUrl == nil {
		return ""
	}
	return strings.TrimPrefix(parsedUrl.Path, "/")
}

// ConnectMany opens one *sql.DB per name in dbNames. On any per-database failure,
// every handle opened so far is closed before returning the error.
func ConnectMany(ctx context.Context, opts ConnectOptions, dbNames []string) (map[string]*sql.DB, DbEngine, error) {
	if len(dbNames) == 0 {
		return nil, Unknown, errors.New("ConnectMany: dbNames is empty")
	}

	dbs := make(map[string]*sql.DB, len(dbNames))
	closeAll := func() {
		for _, opened := range dbs {
			_ = opened.Close()
		}
	}

	var engine DbEngine
	for i, name := range dbNames {
		perOpts := opts
		perOpts.Database = name
		db, eng, err := Connect(ctx, perOpts)
		if err != nil {
			closeAll()
			return nil, Unknown, fmt.Errorf("ConnectMany: failed to connect to database %q: %w", name, err)
		}
		if i == 0 {
			engine = eng
		} else if eng != engine {
			_ = db.Close()
			closeAll()
			return nil, Unknown, fmt.Errorf("ConnectMany: engine mismatch between databases (%v vs %v)", eng, engine)
		}
		dbs[name] = db
	}
	return dbs, engine, nil
}

func Connect(ctx context.Context, opts ConnectOptions) (*sql.DB, DbEngine, error) {
	parsedDsn, err := buildConnectionURL(opts)
	if err != nil {
		return nil, Unknown, err
	}

	if parsedDsn.Scheme == "" {
		return nil, Unknown, errors.New("database scheme must be specified in DSN or configuration")
	}

	switch parsedDsn.Scheme {
	case "mysql":
		db, err := mysql.Connect(ctx, parsedDsn.String())
		if err != nil {
			return nil, Unknown, err
		}
		return db, MySQL, nil

	case "oracle":
		db, err := oracle.Connect(ctx, parsedDsn.String())
		if err != nil {
			return nil, Unknown, err
		}
		return db, Oracle, nil

	case "sqlserver":
		db, err := sqlserver.Connect(ctx, parsedDsn.String())
		if err != nil {
			return nil, Unknown, err
		}
		return db, MSSQL, nil

	case "postgres":
		db, err := postgres.Connect(ctx, parsedDsn.String())
		if err != nil {
			return nil, Unknown, err
		}
		return db, PostgreSQL, nil

	case "hdb":
		db, err := hdb.Connect(ctx, parsedDsn.String())
		if err != nil {
			return nil, Unknown, err
		}
		return db, HDB, nil

	case "vertica":
		db, err := vertica.Connect(ctx, parsedDsn.String())
		if err != nil {
			return nil, Unknown, err
		}
		return db, Vertica, nil

	case "db2":
		db, err := db2.Connect(ctx, parsedDsn.String())
		if err != nil {
			return nil, Unknown, err
		}
		return db, DB2, nil

	default:
		return nil, Unknown, fmt.Errorf("unsupported database scheme: %s", parsedDsn.Scheme)
	}
}

func buildConnectionURL(opts ConnectOptions) (*url.URL, error) {
	var (
		parsedUrl *url.URL
		err       error
	)

	if opts.DSN != "" {
		populatedDSN, err := expandDSN(opts.DSN)
		if err != nil {
			return nil, err
		}
		parsedUrl, err = url.Parse(populatedDSN)
		if err != nil {
			return nil, err
		}
	} else {
		parsedUrl = &url.URL{}
	}

	scheme, err := expandValue(opts.Scheme)
	if err != nil {
		return nil, err
	}
	if scheme != "" {
		parsedUrl.Scheme = scheme
	}

	host, err := expandValue(opts.Host)
	if err != nil {
		return nil, err
	}

	port, err := expandValue(opts.Port)
	if err != nil {
		return nil, err
	}

	hostValue := parsedUrl.Hostname()
	if hostValue == "" && parsedUrl.Host != "" {
		hostValue = parsedUrl.Host
	}
	if host != "" {
		hostValue = host
	}

	portValue := parsedUrl.Port()
	if portValue == "" && parsedUrl.Host != "" {
		if _, p, err := net.SplitHostPort(parsedUrl.Host); err == nil {
			portValue = p
		}
	}
	if port != "" {
		portValue = port
	}

	if hostValue != "" {
		if portValue != "" {
			// JoinHostPort expects the host without brackets for IPv6
			// If hostValue has brackets, strip them; if it's a raw IPv6, leave as-is
			cleanHost := hostValue
			if len(hostValue) > 2 && hostValue[0] == '[' && hostValue[len(hostValue)-1] == ']' {
				cleanHost = hostValue[1 : len(hostValue)-1]
			}
			parsedUrl.Host = net.JoinHostPort(cleanHost, portValue)
		} else {
			parsedUrl.Host = hostValue
		}
	} else if portValue != "" {
		return nil, fmt.Errorf("port provided without host")
	}

	databaseName, err := expandValue(opts.Database)
	if err != nil {
		return nil, err
	}
	if databaseName != "" {
		if strings.HasPrefix(databaseName, "/") {
			parsedUrl.Path = databaseName
		} else {
			parsedUrl.Path = "/" + databaseName
		}
	}

	user, err := expandValue(opts.User)
	if err != nil {
		return nil, err
	}

	password, err := expandValue(opts.Password)
	if err != nil {
		return nil, err
	}

	if user != "" || password != "" {
		if password != "" {
			parsedUrl.User = url.UserPassword(user, password)
		} else {
			parsedUrl.User = url.User(user)
		}
	}

	if len(opts.Params) > 0 {
		values := parsedUrl.Query()
		if values == nil {
			values = url.Values{}
		}
		for k, v := range opts.Params {
			key, err := expandValue(k)
			if err != nil {
				return nil, err
			}
			value, err := expandValue(v)
			if err != nil {
				return nil, err
			}
			values.Set(key, value)
		}
		parsedUrl.RawQuery = values.Encode()
	}

	if parsedUrl.Scheme == "" && opts.DSN == "" {
		return nil, fmt.Errorf("database scheme must be specified")
	}

	return parsedUrl, nil
}

func expandValue(s string) (string, error) {
	if s == "" {
		return s, nil
	}
	if DSNREnvRegex.MatchString(s) {
		return updateFromEnv(s)
	}
	return s, nil
}
