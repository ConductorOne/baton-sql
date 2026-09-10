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
// authentication/authorization failure, or nil otherwise. name identifies the failing
// database so a multi-DB config still shows which handle rejected the credentials.
// SQLSTATE class 28 ("invalid authorization") is the ANSI code drivers report on bad
// credentials (Postgres/Redshift/Vertica/etc. surface it via SQLState()); MySQL is the
// exception, reporting error 1045 with no SQLSTATE.
//
// Coverage is limited to drivers that expose SQLState() plus MySQL. Drivers that do not
// (Oracle go-ora, Db2 go_ibm_db, MSSQL, SAP HDB) fall through to nil, so their auth
// failures reach the caller as a generic ping error rather than Unauthenticated.
func AuthError(err error, name string) error {
	if err == nil {
		return nil
	}

	var sqlState interface{ SQLState() string }
	if errors.As(err, &sqlState) && strings.HasPrefix(sqlState.SQLState(), "28") {
		return status.Errorf(codes.Unauthenticated, "database %q authentication failed", name)
	}

	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) && myErr.Number == mysqlAccessDenied {
		return status.Errorf(codes.Unauthenticated, "database %q authentication failed", name)
	}

	return nil
}
