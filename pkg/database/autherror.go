package database

import (
	"errors"
	"fmt"
	"strings"

	"github.com/conductorone/baton-sdk/pkg/uhttp"
	"github.com/conductorone/baton-sql/pkg/database/db2"
	"github.com/go-sql-driver/mysql"
	"google.golang.org/grpc/codes"
)

const mysqlAccessDenied = 1045

// AuthError wraps err in an Unauthenticated gRPC status naming the failing database when
// err is a database auth failure, or returns nil otherwise, preserving the original error
// via errors.As. Detection covers SQLSTATE class 28 (Postgres/Redshift), MySQL error 1045,
// and DB2 (db2.IsAuthError); drivers without any of these (Vertica, Oracle, MSSQL, SAP HDB)
// fall through to a generic ping error.
func AuthError(err error, name string) error {
	if err == nil || !isAuthFailure(err) {
		return nil
	}
	return uhttp.WrapErrors(codes.Unauthenticated, fmt.Sprintf("database %q authentication failed", name), err)
}

func isAuthFailure(err error) bool {
	var sqlState interface{ SQLState() string }
	if errors.As(err, &sqlState) && strings.HasPrefix(sqlState.SQLState(), "28") {
		return true
	}

	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) && myErr.Number == mysqlAccessDenied {
		return true
	}

	return db2.IsAuthError(err)
}
