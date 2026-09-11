package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNativeDB2DSN(t *testing.T) {
	const native = "HOSTNAME=localhost;PORT=50000;DATABASE=TESTDB;UID=db2inst1;PWD=pass123;PROTOCOL=TCPIP"

	lookup := func(m map[string]string) LookupFunc {
		return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
	}

	tests := []struct {
		name    string
		opts    ConnectOptions
		wantDSN string
		wantOk  bool
		wantErr string
	}{
		{name: "native form no scheme", opts: ConnectOptions{DSN: native}, wantDSN: native, wantOk: true},
		{name: "native form with scheme db2", opts: ConnectOptions{DSN: native, Scheme: "db2"}, wantDSN: native, wantOk: true},
		// DATABASE without HOSTNAME is a generic ODBC/ADO shape, not native DB2: it must
		// fall through to the normal scheme check rather than route to the DB2 driver.
		{name: "database marker only is not native", opts: ConnectOptions{DSN: "DATABASE=TESTDB;HOST=x"}, wantDSN: "", wantOk: false},
		{
			name:    "lowercase keywords",
			opts:    ConnectOptions{DSN: "hostname=h;port=50000;database=X;uid=u;pwd=p"},
			wantDSN: "hostname=h;port=50000;database=X;uid=u;pwd=p",
			wantOk:  true,
		},
		{
			name:    "whitespace after separators",
			opts:    ConnectOptions{DSN: "HOSTNAME=h; DATABASE=X; UID=u"},
			wantDSN: "HOSTNAME=h; DATABASE=X; UID=u",
			wantOk:  true,
		},
		{
			name:    "value containing :// is not a url",
			opts:    ConnectOptions{DSN: "HOSTNAME=h;DATABASE=X;PWD=my://secret"},
			wantDSN: "HOSTNAME=h;DATABASE=X;PWD=my://secret",
			wantOk:  true,
		},
		{
			name: "native form with placeholders",
			opts: ConnectOptions{
				DSN:    "HOSTNAME=${DB_HOST};PORT=50000;DATABASE=${DB_NAME};UID=u;PWD=p",
				Lookup: lookup(map[string]string{"DB_HOST": "h", "DB_NAME": "d"}),
			},
			wantDSN: "HOSTNAME=h;PORT=50000;DATABASE=d;UID=u;PWD=p",
			wantOk:  true,
		},
		{
			name:    "scheme placeholder expands to db2",
			opts:    ConnectOptions{DSN: native, Scheme: "${SCH}", Lookup: lookup(map[string]string{"SCH": "db2"})},
			wantDSN: native,
			wantOk:  true,
		},
		{name: "db2 url form", opts: ConnectOptions{DSN: "db2://u:p@h:50000/db"}, wantOk: false},
		{name: "postgres url form", opts: ConnectOptions{DSN: "postgres://h/db"}, wantOk: false},
		{name: "native markers but foreign scheme", opts: ConnectOptions{DSN: native, Scheme: "postgres"}, wantOk: false},
		{name: "url with database marker in query", opts: ConnectOptions{DSN: "db2://h:50000/db?DATABASE=x"}, wantOk: false},
		{name: "empty dsn", opts: ConnectOptions{}, wantOk: false},
		{
			name:    "unset placeholder errors",
			opts:    ConnectOptions{DSN: "HOSTNAME=${MISSING};DATABASE=d", Lookup: lookup(map[string]string{})},
			wantErr: "MISSING",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDSN, _, gotOk, err := nativeDB2DSN(tt.opts)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantOk, gotOk)
			require.Equal(t, tt.wantDSN, gotDSN)
		})
	}
}

func TestResolveDatabaseNameNativeDB2(t *testing.T) {
	// The native form must resolve the same database name as the equivalent db2:// URL,
	// so resource IDs stay stable across the two DSN forms.
	tests := []struct {
		name string
		opts ConnectOptions
		want string
	}{
		{
			name: "native form",
			opts: ConnectOptions{DSN: "HOSTNAME=h;PORT=50000;DATABASE=TESTDB;UID=u;PWD=p;PROTOCOL=TCPIP"},
			want: "TESTDB",
		},
		{
			name: "native form braced database",
			opts: ConnectOptions{DSN: "HOSTNAME=h;DATABASE={my;db};UID=u"},
			want: "my;db",
		},
		{
			name: "lowercase database keyword",
			opts: ConnectOptions{DSN: "hostname=h;database=testdb;uid=u"},
			want: "testdb",
		},
		{
			name: "whitespace before database keyword",
			opts: ConnectOptions{DSN: "HOSTNAME=h; DATABASE=TESTDB; UID=u"},
			want: "TESTDB",
		},
		{
			name: "equivalent url form",
			opts: ConnectOptions{DSN: "db2://u:p@h:50000/TESTDB"},
			want: "TESTDB",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ResolveDatabaseName(tt.opts))
		})
	}
}

func TestExpandNativeDSN(t *testing.T) {
	lookup := func(k string) (string, bool) {
		m := map[string]string{"H": "dbhost", "PW": "secret", "BAD": "x;DATABASE=other"}
		v, ok := m[k]
		return v, ok
	}

	got, err := expandNativeDSN("HOSTNAME=${H};PWD=${PW}", lookup)
	require.NoError(t, err)
	require.Equal(t, "HOSTNAME=dbhost;PWD=secret", got)

	// A placeholder value carrying ODBC separators can't inject extra keywords.
	_, err = expandNativeDSN("HOSTNAME=${H};PWD=${BAD}", lookup)
	require.ErrorContains(t, err, "ODBC keyword separators")

	// A single ${KEY} spanning the whole DSN is the full value, so its separators are kept.
	whole := func(k string) (string, bool) { return "HOSTNAME=h;DATABASE=d", k == "DSN" }
	got, err = expandNativeDSN("${DSN}", whole)
	require.NoError(t, err)
	require.Equal(t, "HOSTNAME=h;DATABASE=d", got)

	_, err = expandNativeDSN("HOSTNAME=${MISSING}", lookup)
	require.ErrorContains(t, err, "is not set")
}
