package db2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConvertToDB2DSN(t *testing.T) {
	tests := []struct {
		name    string
		dsn     string
		want    string
		wantErr string
	}{
		{
			name: "url with credentials and port",
			dsn:  "db2://user:pass@dbhost:50001/testdb",
			want: "HOSTNAME=dbhost;DATABASE=testdb;PORT=50001;PROTOCOL=TCPIP;UID=user;PWD=pass",
		},
		{
			name: "url without port uses default",
			dsn:  "db2://dbhost/testdb",
			want: "HOSTNAME=dbhost;DATABASE=testdb;PORT=50000;PROTOCOL=TCPIP",
		},
		{
			name: "native dsn passed through",
			dsn:  "HOSTNAME=dbhost;PORT=50000;DATABASE=testdb;UID=user;PWD=pass",
			want: "HOSTNAME=dbhost;PORT=50000;DATABASE=testdb;UID=user;PWD=pass",
		},
		{
			name: "db2i scheme without port uses DRDA default 446",
			dsn:  "db2i://user:pass@ibmi/MYRDB",
			want: "HOSTNAME=ibmi;DATABASE=MYRDB;PORT=446;PROTOCOL=TCPIP;UID=user;PWD=pass",
		},
		{
			name: "db2i scheme with explicit port",
			dsn:  "db2i://user:pass@ibmi:8471/MYRDB",
			want: "HOSTNAME=ibmi;DATABASE=MYRDB;PORT=8471;PROTOCOL=TCPIP;UID=user;PWD=pass",
		},
		{
			name: "protocol overridable via query param (no duplicate default)",
			dsn:  "db2://user:pass@dbhost:50000/testdb?PROTOCOL=IPC",
			want: "HOSTNAME=dbhost;DATABASE=testdb;PORT=50000;UID=user;PWD=pass;PROTOCOL=IPC",
		},
		{
			name:    "wrong scheme",
			dsn:     "postgres://dbhost/testdb",
			wantErr: "expected db2:// or db2i:// scheme",
		},
		{
			name:    "missing database name",
			dsn:     "db2://dbhost:50000",
			wantErr: "database name is required",
		},
		{
			name: "password with semicolon is brace-quoted",
			dsn:  "db2://user:my;pass@dbhost:50000/testdb",
			want: "HOSTNAME=dbhost;DATABASE=testdb;PORT=50000;PROTOCOL=TCPIP;UID=user;PWD={my;pass}",
		},
		{
			name: "url-encoded password decoded then brace-quoted",
			dsn:  "db2://user:p%3Bss%20word@dbhost:50000/testdb",
			want: "HOSTNAME=dbhost;DATABASE=testdb;PORT=50000;PROTOCOL=TCPIP;UID=user;PWD={p;ss word}",
		},
		{
			name:    "password with closing brace rejected",
			dsn:     "db2://user:pa%7Dss@dbhost:50000/testdb",
			wantErr: "invalid password",
		},
		{
			name:    "query parameters colliding after uppercasing rejected",
			dsn:     "db2://user:pass@dbhost:50000/testdb?security=SSL&SECURITY=NONE",
			wantErr: "specified multiple times",
		},
		{
			name: "url dsn containing native marker in password still parsed as url",
			dsn:  "db2://user:DATABASE=x@dbhost:50000/testdb",
			want: "HOSTNAME=dbhost;DATABASE=testdb;PORT=50000;PROTOCOL=TCPIP;UID=user;PWD={DATABASE=x}",
		},
		{
			name: "hostname with semicolon is brace-quoted",
			dsn:  "db2://user:pass@evil;SECURITY=NONE/testdb",
			want: "HOSTNAME={evil;SECURITY=NONE};DATABASE=testdb;PORT=50000;PROTOCOL=TCPIP;UID=user;PWD=pass",
		},
		{
			name: "ipv6 hostname passes through unquoted",
			dsn:  "db2://user:pass@[::1]:50000/testdb",
			want: "HOSTNAME=::1;DATABASE=testdb;PORT=50000;PROTOCOL=TCPIP;UID=user;PWD=pass",
		},
		{
			name: "query parameters forwarded as keywords sorted",
			dsn:  "db2://user:pass@dbhost:50000/testdb?Security=SSL&CurrentSchema=MYSCHEMA",
			want: "HOSTNAME=dbhost;DATABASE=testdb;PORT=50000;PROTOCOL=TCPIP;UID=user;PWD=pass;CURRENTSCHEMA=MYSCHEMA;SECURITY=SSL",
		},
		{
			name:    "query parameter overriding reserved keyword rejected",
			dsn:     "db2://user:pass@dbhost:50000/testdb?uid=other",
			wantErr: "conflicts with a reserved DSN keyword",
		},
		{
			name:    "duplicate query parameter rejected",
			dsn:     "db2://user:pass@dbhost:50000/testdb?Security=SSL&Security=NONE",
			wantErr: "specified multiple times",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convertToDB2DSN(tt.dsn)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
