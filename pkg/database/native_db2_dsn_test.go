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
		{name: "database marker only", opts: ConnectOptions{DSN: "DATABASE=TESTDB;HOST=x"}, wantDSN: "DATABASE=TESTDB;HOST=x", wantOk: true},
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
			gotDSN, gotOk, err := nativeDB2DSN(tt.opts)
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
