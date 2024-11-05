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
)

var DSNREnvRegex = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

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

func Connect(ctx context.Context, dsn string) (*sql.DB, string, error) {
	populatedDSN, err := updateDSNFromEnv(ctx, dsn)
	if err != nil {
		return nil, "", err
	}

	parsedDsn, err := url.Parse(populatedDSN)
	if err != nil {
		return nil, "", err
	}

	switch parsedDsn.Scheme {
	case "mysql":
		db, err := mysql.Connect(ctx, populatedDSN)
		if err != nil {
			return nil, "", err
		}
		return db, "mysql", nil
	default:
		return nil, "", fmt.Errorf("unsupported database scheme: %s", parsedDsn.Scheme)
	}
}
