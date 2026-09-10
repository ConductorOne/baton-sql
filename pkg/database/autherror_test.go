package database

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAuthError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want codes.Code // codes.OK means expect nil
	}{
		{"nil", nil, codes.OK},
		{"postgres invalid_password 28P01", &pgconn.PgError{Code: "28P01"}, codes.Unauthenticated},
		{"postgres invalid_authorization 28000", &pgconn.PgError{Code: "28000"}, codes.Unauthenticated},
		{"postgres non-auth relation missing 42P01", &pgconn.PgError{Code: "42P01"}, codes.OK},
		{"postgres auth error wrapped", fmt.Errorf("ping: %w", &pgconn.PgError{Code: "28P01"}), codes.Unauthenticated},
		{"mysql access denied 1045", &mysql.MySQLError{Number: 1045}, codes.Unauthenticated},
		{"mysql other 1146", &mysql.MySQLError{Number: 1146}, codes.OK},
		{"plain error", errors.New("boom"), codes.OK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AuthError(tt.err, "testdb")
			if tt.want == codes.OK {
				if got != nil {
					t.Fatalf("want nil, got %v", got)
				}
				return
			}
			if status.Code(got) != tt.want {
				t.Fatalf("want %v, got %v", tt.want, status.Code(got))
			}
		})
	}
}
