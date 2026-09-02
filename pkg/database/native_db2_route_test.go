//go:build !db2

package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// A native DB2 DSN set through config must reach the DB2 driver, not be rejected by the
// URL builder. On a default (non-db2) build that means Connect returns the "not compiled"
// stub error, never the "scheme must be specified" / "database name is required" errors
// the URL builder raises for an opaque DSN.
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
