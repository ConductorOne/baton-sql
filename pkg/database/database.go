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

	"github.com/conductorone/baton-sql/pkg/database/hdb"
	"github.com/conductorone/baton-sql/pkg/database/mysql"
	"github.com/conductorone/baton-sql/pkg/database/oracle"
	"github.com/conductorone/baton-sql/pkg/database/postgres"
	"github.com/conductorone/baton-sql/pkg/database/sqlserver"
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

// extractPlaceholders replaces ${...} placeholders with unique numeric sentinels
// and looks up the environment variable values immediately.
// Numeric sentinels (999000, 999001, etc.) are valid in all URL components:
// ports (must be numeric), hostnames, userinfo, paths, and query strings.
// This allows us to parse the URL structure before expanding environment variables.
// Returns: the string with sentinels, a mapping of sentinel->value, and any error.
func extractPlaceholders(s string) (string, map[string]string, error) {
	mapping := make(map[string]string)
	counter := 0
	var err error

	result := DSNREnvRegex.ReplaceAllStringFunc(s, func(match string) string {
		sentinel := fmt.Sprintf("999%03d", counter)
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
// corresponding values from the mapping.
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
func expandHost(parsedUrl *url.URL, mapping map[string]string) {
	if parsedUrl.Host != "" {
		parsedUrl.Host = expandWithMapping(parsedUrl.Host, mapping)
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
