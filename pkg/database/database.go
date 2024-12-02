package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"

	"github.com/conductorone/baton-sql/pkg/database/mysql"
	"github.com/conductorone/baton-sql/pkg/database/oracle"
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
)

func updateDSNFromEnv(ctx context.Context, dsn string) (string, error) {
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

func Connect(ctx context.Context, dsn string) (*sql.DB, DbEngine, error) {
	populatedDSN, err := updateDSNFromEnv(ctx, dsn)
	if err != nil {
		return nil, Unknown, err
	}

	parsedDsn, err := url.Parse(populatedDSN)
	if err != nil {
		return nil, Unknown, err
	}

	switch parsedDsn.Scheme {
	case "mysql":
		db, err := mysql.Connect(ctx, populatedDSN)
		if err != nil {
			return nil, Unknown, err
		}
		return db, MySQL, nil

	case "oracle":
		db, err := oracle.Connect(ctx, populatedDSN)
		if err != nil {
			return nil, Unknown, err
		}
		return db, Oracle, nil
	default:
		return nil, Unknown, fmt.Errorf("unsupported database scheme: %s", parsedDsn.Scheme)
	}
}
