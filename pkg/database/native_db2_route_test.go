//go:build !db2

package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// A native DB2 DSN must reach the DB2 driver, not the URL builder; on a default (non-db2)
// build, Connect should return the "not compiled" stub error, never the URL builder's
// scheme/database errors.
func TestConnectNativeDB2DSNReachesDriver(t *testing.T) {
	const native = "HOSTNAME=localhost;PORT=50000;DATABASE=TESTDB;UID=db2inst1;PWD=pass123;PROTOCOL=TCPIP"

	for _, tt := range []struct {
		name string
		opts ConnectOptions
	}{
		{name: "no scheme", opts: ConnectOptions{DSN: native}},
		{name: "scheme db2", opts: ConnectOptions{DSN: native, Scheme: "db2"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := Connect(context.Background(), tt.opts)
			require.Error(t, err)
			require.ErrorContains(t, err, "DB2 support not compiled")
			require.NotContains(t, err.Error(), "scheme must be specified")
			require.NotContains(t, err.Error(), "database name is required")
		})
	}
}

// A native DSN already carries every connection setting, so pairing it with structured
// fields or a per-database override must be rejected up front, never silently dropped —
// including via ConnectMany's per-name Database override.
func TestConnectNativeDB2DSNRejectsStructuredFields(t *testing.T) {
	const native = "HOSTNAME=localhost;PORT=50000;DATABASE=TESTDB;UID=db2inst1;PWD=pass123;PROTOCOL=TCPIP"

	for _, tt := range []struct {
		name string
		opts ConnectOptions
	}{
		{name: "database override", opts: ConnectOptions{DSN: native, Database: "OTHERDB"}},
		{name: "host", opts: ConnectOptions{DSN: native, Host: "elsewhere"}},
		{name: "port", opts: ConnectOptions{DSN: native, Port: "50001"}},
		{name: "user", opts: ConnectOptions{DSN: native, User: "someone"}},
		{name: "password", opts: ConnectOptions{DSN: native, Password: "secret"}},
		{name: "params", opts: ConnectOptions{DSN: native, Params: map[string]string{"SECURITY": "SSL"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := Connect(context.Background(), tt.opts)
			require.Error(t, err)
			require.ErrorContains(t, err, "self-contained")
			require.NotContains(t, err.Error(), "DB2 support not compiled")
		})
	}

	t.Run("multi-database via ConnectMany", func(t *testing.T) {
		_, _, err := ConnectMany(context.Background(), ConnectOptions{DSN: native}, []string{"A", "B"})
		require.Error(t, err)
		require.ErrorContains(t, err, "self-contained")
	})
}
