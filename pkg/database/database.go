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
)

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

func Connect(ctx context.Context, dsn string, user string, password string) (*sql.DB, DbEngine, error) {
	populatedDSN, err := updateFromEnv(dsn)
	if err != nil {
		return nil, Unknown, err
	}

	parsedDsn, err := url.Parse(populatedDSN)
	if err != nil {
		return nil, Unknown, err
	}

	if parsedDsn.User == nil {
		if user == "" || password == "" {
			return nil, Unknown, errors.New("user and password must be set in DSN or in the configuration")
		}

		populatedUser, err := updateFromEnv(user)
		if err != nil {
			return nil, Unknown, err
		}

		populatedPassword, err := updateFromEnv(password)
		if err != nil {
			return nil, Unknown, err
		}

		parsedDsn.User = url.UserPassword(populatedUser, populatedPassword)
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
	default:
		return nil, Unknown, fmt.Errorf("unsupported database scheme: %s", parsedDsn.Scheme)
	}
}
