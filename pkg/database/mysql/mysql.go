package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

func convertURItoDSN(uri string) (string, error) {
	parsedURI, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("failed to parse URI: %w", err)
	}

	if parsedURI.Scheme != "mysql" {
		return "", fmt.Errorf("invalid URI scheme: %s, expected mysql", parsedURI.Scheme)
	}

	user := ""
	password := ""
	if parsedURI.User != nil {
		user = parsedURI.User.Username()
		password, _ = parsedURI.User.Password()
	}

	host := parsedURI.Host
	if !strings.Contains(host, ":") {
		host += ":3306" // Default to port 3306 if not specified
	}

	dbname := strings.TrimPrefix(parsedURI.Path, "/")

	queryParams := parsedURI.RawQuery

	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s", user, password, host, dbname)
	if queryParams != "" {
		dsn += "?" + queryParams
	}

	return dsn, nil
}

func Connect(ctx context.Context, dsn string) (*sql.DB, error) {
	connectDSN, err := convertURItoDSN(dsn)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("mysql", connectDSN)
	if err != nil {
		return nil, err
	}

	return db, nil
}
