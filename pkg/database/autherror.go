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

// AuthError wraps err in an Unauthenticated gRPC status when it is a database
// authentication/authorization failure, or returns nil otherwise. name identifies the
// failing database so a multi-DB config still shows which handle rejected the
// credentials. Each driver surfaces bad credentials differently: SQLSTATE class 28 via a
// SQLState() method (Postgres/Redshift), MySQL error 1045 with no SQLSTATE, and DB2 in a
// driver-specific field (see db2.IsAuthError). Drivers that expose SQLSTATE only as a
// struct field (Vertica) or not at all (Oracle, MSSQL, SAP HDB) are not covered and fall
// through to a generic ping error. Wrapping (rather than status.Errorf) keeps the
// original error reachable via errors.As.
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
