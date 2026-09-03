package database

import (
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const mysqlAccessDenied = 1045

// AuthError returns an Unauthenticated gRPC status when err is a database
// authentication/authorization failure, or nil otherwise. SQLSTATE class 28
// ("invalid authorization") is the ANSI code drivers report on bad credentials
// (Postgres/Redshift/Vertica/etc. surface it via SQLState()); MySQL is the
// exception, reporting error 1045 with no SQLSTATE.
func AuthError(err error) error {
	if err == nil {
		return nil
	}

	var sqlState interface{ SQLState() string }
	if errors.As(err, &sqlState) && strings.HasPrefix(sqlState.SQLState(), "28") {
		return status.Error(codes.Unauthenticated, "database authentication failed")
	}

	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) && myErr.Number == mysqlAccessDenied {
		return status.Error(codes.Unauthenticated, "database authentication failed")
	}

	return nil
}
