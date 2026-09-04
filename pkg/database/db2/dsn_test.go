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
			name: "lowercase native dsn passed through",
			dsn:  "hostname=dbhost;port=50000;database=testdb;uid=user;pwd=pass",
			want: "hostname=dbhost;port=50000;database=testdb;uid=user;pwd=pass",
		},
		{
			name: "native dsn with whitespace passed through",
			dsn:  "HOSTNAME=dbhost; DATABASE=testdb; UID=user",
			want: "HOSTNAME=dbhost; DATABASE=testdb; UID=user",
		},
		{
			name:    "wrong scheme",
			dsn:     "postgres://dbhost/testdb",
			wantErr: "expected db2:// scheme",
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

func TestIsNativeDSN(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want bool
	}{
		{name: "native markers", dsn: "HOSTNAME=h;DATABASE=X", want: true},
		{name: "lowercase keywords", dsn: "hostname=h;database=x", want: true},
		{name: "whitespace after separator", dsn: "HOSTNAME=h; DATABASE=X", want: true},
		{name: "db2 url", dsn: "db2://u:p@h:50000/db", want: false},
		{name: "postgres url", dsn: "postgres://h/db", want: false},
		{name: "value carrying :// is not a url", dsn: "HOSTNAME=h;PWD=my://secret", want: true},
		{name: "space before the =", dsn: "DATABASE = X", want: true},
		// DATABASE= appears only inside a braced PWD value, so the brace-aware split keeps it
		// as one PWD part: not a native marker. Routing and passthrough now agree here.
		{name: "database marker only inside braced value", dsn: "UID=u;PWD={x;DATABASE=y}", want: false},
		// Unterminated '{' is literal, so the ';' still splits and DATABASE= stays visible;
		// the malformed value then reaches the driver instead of silently misrouting.
		{name: "unterminated brace keeps marker visible", dsn: "PWD={oops;DATABASE=X", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsNativeDSN(tt.dsn))
		})
	}
}

func TestDSNDatabase(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{name: "plain", dsn: "HOSTNAME=h;DATABASE=TESTDB;UID=u", want: "TESTDB"},
		{name: "braced value with semicolon", dsn: "HOSTNAME=h;DATABASE={my;db}", want: "my;db"},
		{name: "lowercase", dsn: "hostname=h;database=testdb", want: "testdb"},
		{name: "whitespace before keyword", dsn: "HOSTNAME=h; DATABASE=TESTDB", want: "TESTDB"},
		{name: "space after the =", dsn: "HOSTNAME=h;DATABASE= TESTDB", want: "TESTDB"},
		{name: "space before the =", dsn: "HOSTNAME=h;DATABASE = TESTDB", want: "TESTDB"},
		// Space between '=' and a braced value must still brace-detect, else the ';'
		// inside the braces splits and the database name comes back truncated.
		{name: "space before braced value", dsn: "HOSTNAME=h;DATABASE= {my;db}", want: "my;db"},
		// A literal '{' mid-value (not ODBC quoting) must not swallow the following ';'.
		{name: "unquoted brace in earlier value", dsn: "HOSTNAME=h;PWD=p{q;DATABASE=TESTDB", want: "TESTDB"},
		{name: "absent", dsn: "HOSTNAME=h;UID=u", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, DSNDatabase(tt.dsn))
		})
	}
}
