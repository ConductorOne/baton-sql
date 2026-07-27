//nolint:gosec // Tests contain example passwords.
package database

import (
	"context"
	"os"
	"strings"
	"testing"
)

func Test_ConnectMany_RejectsEmptyList(t *testing.T) {
	_, _, err := ConnectMany(t.Context(), ConnectOptions{Scheme: "postgres"}, nil)
	if err == nil {
		t.Fatal("expected error for empty dbNames")
	}
	if !strings.Contains(err.Error(), "dbNames is empty") {
		t.Errorf("expected dbNames error, got: %v", err)
	}
}

func Test_ConnectMany_UnsupportedSchemePropagates(t *testing.T) {
	// Use an unsupported scheme so Connect rejects each opts before any *sql.DB is opened.
	// ConnectMany must propagate the error rather than swallow it.
	_, _, err := ConnectMany(t.Context(), ConnectOptions{Scheme: "unsupported-engine"}, []string{"a", "b"})
	if err == nil {
		t.Fatal("expected unsupported-scheme error")
	}
	if !strings.Contains(err.Error(), "unsupported database scheme") {
		t.Errorf("expected scheme error, got: %v", err)
	}
}

func Test_updateDSNFromEnv(t *testing.T) {
	type args struct {
		ctx context.Context
		dsn string
	}
	tests := []struct {
		name    string
		env     map[string]string
		args    args
		want    string
		wantErr bool
	}{
		{
			"Test valid DSN with no replacements",
			map[string]string{},
			args{
				t.Context(),
				"mysql://user:password@localhost:3306/dbname",
			},
			"mysql://user:password@localhost:3306/dbname",
			false,
		},
		{
			"Test valid DSN with all replacements in env",
			map[string]string{
				"DB_USER":     "user",
				"DB_PASSWORD": "password",
				"DB_HOST":     "localhost",
				"DB_PORT":     "3306",
				"DB_NAME":     "dbname",
			},
			args{
				t.Context(),
				"mysql://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}",
			},
			"mysql://user:password@localhost:3306/dbname",
			false,
		},
		{
			"Test valid DSN with replacement from env missing",
			map[string]string{
				"DB_USER":     "user",
				"DB_PASSWORD": "password",
				"DB_HOST":     "localhost",
				"DB_NAME":     "dbname",
			},
			args{
				t.Context(),
				"mysql://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}",
			},
			"",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got, err := updateFromEnv(tt.args.dsn)
			if (err != nil) != tt.wantErr {
				t.Errorf("updateFromEnv() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("updateFromEnv() got = %v, want %v", got, tt.want)
			}
			for k := range tt.env {
				if err := os.Unsetenv(k); err != nil {
					t.Fatalf("failed to unset env var %s: %v", k, err)
				}
			}
		})
	}
}

func Test_expandDSN(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		dsn     string
		want    string
		wantErr bool
	}{
		{
			name: "No placeholders",
			env:  map[string]string{},
			dsn:  "mysql://user:password@localhost:3306/dbname",
			want: "mysql://user:password@localhost:3306/dbname",
		},
		{
			name: "Simple password with hash symbol",
			env: map[string]string{
				"DB_PASSWORD": "p@ss#123",
			},
			dsn:  "mysql://admin:${DB_PASSWORD}@localhost:3306/dbname",
			want: "mysql://admin:p%40ss%23123@localhost:3306/dbname",
		},
		{
			name: "Port",
			env: map[string]string{
				"DB_PORT": "3306",
			},
			dsn:  "mysql://user:password@localhost:${DB_PORT}/dbname",
			want: "mysql://user:password@localhost:3306/dbname",
		},
		{
			name: "Password with multiple special characters",
			env: map[string]string{
				"DB_PASSWORD": "p@ss#wo:rd/123",
			},
			dsn:  "mysql://admin:${DB_PASSWORD}@localhost:3306/dbname",
			want: "mysql://admin:p%40ss%23wo%3Ard%2F123@localhost:3306/dbname",
		},
		{
			name: "Username with special characters",
			env: map[string]string{
				"DB_USER": "user@domain",
			},
			dsn:  "mysql://${DB_USER}:password@localhost:3306/dbname",
			want: "mysql://user%40domain:password@localhost:3306/dbname",
		},
		{
			name: "Both username and password with special chars",
			env: map[string]string{
				"DB_USER": "user:name",
				"DB_PASS": "pass#word",
			},
			dsn:  "mysql://${DB_USER}:${DB_PASS}@localhost:3306/dbname",
			want: "mysql://user%3Aname:pass%23word@localhost:3306/dbname",
		},
		{
			name: "Hostname from env variable",
			env: map[string]string{
				"DB_HOST": "db.example.com",
			},
			dsn:  "mysql://admin:password@${DB_HOST}:3306/dbname",
			want: "mysql://admin:password@db.example.com:3306/dbname",
		},
		{
			name: "Port from env variable",
			env: map[string]string{
				"DB_PORT": "3306",
			},
			dsn:  "mysql://user:password@localhost:${DB_PORT}/dbname",
			want: "mysql://user:password@localhost:3306/dbname",
		},
		{
			name: "All components from env including port",
			env: map[string]string{
				"DB_USER": "admin",
				"DB_PASS": "p@ss#123",
				"DB_HOST": "db.example.com:3306",
				"DB_NAME": "mydb",
			},
			dsn:  "mysql://${DB_USER}:${DB_PASS}@${DB_HOST}/${DB_NAME}",
			want: "mysql://admin:p%40ss%23123@db.example.com:3306/mydb",
		},
		{
			name: "Entire userinfo as single variable",
			env: map[string]string{
				"DB_CREDENTIALS": "admin:p@ss#123",
			},
			dsn:  "mysql://${DB_CREDENTIALS}@localhost:3306/dbname",
			want: "mysql://admin:p%40ss%23123@localhost:3306/dbname",
		},
		{
			name: "Same variable used multiple times",
			env: map[string]string{
				"DB_NAME": "mydb",
			},
			dsn:  "mysql://user:pass@localhost:3306/${DB_NAME}?schema=${DB_NAME}",
			want: "mysql://user:pass@localhost:3306/mydb?schema=mydb",
		},
		{
			name: "Query parameters with special chars",
			env: map[string]string{
				"QUERY_VAL": "value&special",
			},
			dsn:  "postgres://user:pass@localhost/db?param=${QUERY_VAL}",
			want: "postgres://user:pass@localhost/db?param=value%26special",
		},
		{
			name: "Space in query parameter value",
			env: map[string]string{
				"PARAM_VALUE": "foo bar",
			},
			dsn:  "postgres://user:pass@localhost/db?setting=${PARAM_VALUE}",
			want: "postgres://user:pass@localhost/db?setting=foo+bar",
		},
		{
			name: "Path with special characters",
			env: map[string]string{
				"DB_NAME": "my/db",
			},
			dsn:  "mysql://user:pass@localhost:3306/${DB_NAME}",
			want: "mysql://user:pass@localhost:3306/my/db",
		},
		{
			name: "PostgreSQL style DSN",
			env: map[string]string{
				"DB_PASSWORD": "test#pass",
			},
			dsn:  "postgres://postgres:${DB_PASSWORD}@localhost:5432/testdb?sslmode=disable",
			want: "postgres://postgres:test%23pass@localhost:5432/testdb?sslmode=disable",
		},
		{
			name: "SQL Server style DSN",
			env: map[string]string{
				"DB_PASSWORD": "P@ss#123",
			},
			dsn:  "sqlserver://sa:${DB_PASSWORD}@localhost:1433?database=master",
			want: "sqlserver://sa:P%40ss%23123@localhost:1433?database=master",
		},
		{
			name: "Oracle style DSN",
			env: map[string]string{
				"DB_PASSWORD": "Oracle#123",
			},
			dsn:  "oracle://system:${DB_PASSWORD}@localhost:1521/ORCLPDB1",
			want: "oracle://system:Oracle%23123@localhost:1521/ORCLPDB1",
		},
		{
			name: "HDB style DSN",
			env: map[string]string{
				"DB_PASSWORD": "hdb#pass",
			},
			dsn:  "hdb://SYSTEM:${DB_PASSWORD}@localhost:39017",
			want: "hdb://SYSTEM:hdb%23pass@localhost:39017",
		},
		{
			name: "Vertica style DSN",
			env: map[string]string{
				"DB_PASSWORD": "vertica#pass",
			},
			dsn:  "vertica://dbadmin:${DB_PASSWORD}@localhost:5433/batondb?tlsmode=none",
			want: "vertica://dbadmin:vertica%23pass@localhost:5433/batondb?tlsmode=none",
		},
		{
			name:    "Missing environment variable",
			env:     map[string]string{},
			dsn:     "mysql://admin:${MISSING_VAR}@localhost:3306/dbname",
			want:    "",
			wantErr: true,
		},
		{
			name: "Empty password",
			env: map[string]string{
				"DB_PASSWORD": "",
			},
			dsn:  "mysql://admin:${DB_PASSWORD}@localhost:3306/dbname",
			want: "mysql://admin:@localhost:3306/dbname",
		},
		{
			name: "Test full DSN",
			env: map[string]string{
				"DSN": "mysql://admin:1234@localhost:3306/dbname",
			},
			dsn:  "${DSN}",
			want: "mysql://admin:1234@localhost:3306/dbname",
		},
		{
			name: "Test DSN parts",
			env: map[string]string{
				"DB_ENDPOINT": "localhost:3306/dbname",
			},
			dsn:  "mysql://admin:password@${DB_ENDPOINT}",
			want: "mysql://admin:password@localhost:3306/dbname",
		},
		{
			name: "Test scheme",
			env: map[string]string{
				"DB_SCHEME": "mysql",
			},
			dsn:  "${DB_SCHEME}://admin:1234@localhost:3306/dbname",
			want: "mysql://admin:1234@localhost:3306/dbname",
		},
		{
			name: "Test replacing",
			env: map[string]string{
				"DB_PASSWORD": "1234",
			},
			dsn:  "mysql://admin:${DB_PASSWORD}@localhost:3306/db-999000",
			want: "mysql://admin:1234@localhost:3306/db-999000",
		},
		{
			name: "Test replacing with port",
			env: map[string]string{
				"DB_PASSWORD": "1234",
				"DB_PORT":     "3306",
			},
			dsn:  "mysql://admin:${DB_PASSWORD}@localhost:${DB_PORT}/db-999001",
			want: "mysql://admin:1234@localhost:3306/db-999001",
		},
		{
			name: "Test replacing with multiple variables",
			env: map[string]string{
				"DB_USER":     "user-999001",
				"DB_PASSWORD": "1234",
			},
			dsn:  "mysql://${DB_USER}:9${DB_PASSWORD}@localhost:3306/dbname",
			want: "mysql://user-999001:91234@localhost:3306/dbname",
		},
		{
			name: "URL with fragment",
			env: map[string]string{
				"DB_FRAGMENT": "section",
			},
			dsn:  "mysql://user:pass@localhost/db#${DB_FRAGMENT}",
			want: "mysql://user:pass@localhost/db#section",
		},
		{
			name: "Complex real-world example",
			env: map[string]string{
				"DB_USER":     "app_user",
				"DB_PASSWORD": "C0mpl3x!P@ss#w0rd$123",
				"DB_HOST":     "prod-db.example.com",
				"DB_NAME":     "production_db",
			},
			dsn:  "postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:5432/${DB_NAME}?sslmode=require&connect_timeout=10",
			want: "postgres://app_user:C0mpl3x%21P%40ss%23w0rd$123@prod-db.example.com:5432/production_db?connect_timeout=10&sslmode=require",
		},
		// Edge cases - security and parser confusion
		{
			name: "Sentinel collision in password",
			env: map[string]string{
				"DB_PASSWORD": "hack__PH_0__attack",
			},
			dsn:  "mysql://admin:${DB_PASSWORD}@localhost/db",
			want: "mysql://admin:hack__PH_0__attack@localhost/db",
		},
		{
			name: "Percent sign in password (literal)",
			env: map[string]string{
				"DB_PASSWORD": "50%off",
			},
			dsn:  "mysql://admin:${DB_PASSWORD}@localhost/db",
			want: "mysql://admin:50%25off@localhost/db",
		},
		{
			name: "At-sign in username",
			env: map[string]string{
				"DB_USER": "user@domain.com",
			},
			dsn:     "mysql://${DB_USER}:password@localhost/db",
			want:    "mysql://user%40domain.com:password@localhost/db",
			wantErr: false, // Should encode the @ properly
		},
		{
			name: "IPv6 address",
			env: map[string]string{
				"DB_HOST": "[2001:db8::1]",
			},
			dsn:  "mysql://user:pass@${DB_HOST}:3306/db",
			want: "mysql://user:pass@[2001:db8::1]:3306/db",
		},
		{
			name: "Empty username with password",
			env: map[string]string{
				"DB_PASSWORD": "secret",
			},
			dsn:  "mysql://:${DB_PASSWORD}@localhost/db",
			want: "mysql://:secret@localhost/db",
		},
		{
			name: "Literal ${ without closing brace",
			env: map[string]string{
				"DB_PASSWORD": "my${incomplete",
			},
			dsn:  "mysql://user:${DB_PASSWORD}@localhost/db",
			want: "mysql://user:my$%7Bincomplete@localhost/db",
		},
		{
			name: "Multiple colons in password",
			env: map[string]string{
				"DB_PASSWORD": "pass:word:with:colons",
			},
			dsn:  "mysql://user:${DB_PASSWORD}@localhost/db",
			want: "mysql://user:pass%3Aword%3Awith%3Acolons@localhost/db",
		},
		{
			name: "Multiple colons in entire userinfo variable",
			env: map[string]string{
				"DB_CREDENTIALS": "user:pass:word:extra",
			},
			dsn:  "mysql://${DB_CREDENTIALS}@localhost/db",
			want: "mysql://user:pass%3Aword%3Aextra@localhost/db",
		},
		{
			name: "Ampersand in password (allowed per RFC 3986)",
			env: map[string]string{
				"DB_PASSWORD": "pass&word",
			},
			dsn:  "mysql://user:${DB_PASSWORD}@localhost/db",
			want: "mysql://user:pass&word@localhost/db",
		},
		{
			name: "Ampersand in query parameter value",
			env: map[string]string{
				"PARAM_VALUE": "value&sneaky",
			},
			dsn:  "postgres://user:pass@localhost/db?setting=${PARAM_VALUE}",
			want: "postgres://user:pass@localhost/db?setting=value%26sneaky",
		},
		{
			name: "Equals sign in database name",
			env: map[string]string{
				"DB_NAME": "db=production",
			},
			dsn:  "mysql://user:pass@localhost/${DB_NAME}",
			want: "mysql://user:pass@localhost/db=production",
		},
		{
			name: "Unicode in password",
			env: map[string]string{
				"DB_PASSWORD": "пароль密码",
			},
			dsn:  "mysql://user:${DB_PASSWORD}@localhost/db",
			want: "mysql://user:%D0%BF%D0%B0%D1%80%D0%BE%D0%BB%D1%8C%E5%AF%86%E7%A0%81@localhost/db",
		},
		{
			name: "Emoji in password",
			env: map[string]string{
				"DB_PASSWORD": "🔒secure",
			},
			dsn:  "mysql://user:${DB_PASSWORD}@localhost/db",
			want: "mysql://user:%F0%9F%94%92secure@localhost/db",
		},
		{
			name: "Backslash in host (SQL Server named instance - gets encoded)",
			env: map[string]string{
				"DB_HOST": "server\\SQLEXPRESS",
			},
			dsn:  "sqlserver://user:pass@${DB_HOST}/db",
			want: "sqlserver://user:pass@server%5CSQLEXPRESS/db",
		},
		{
			name: "Forward slash in password",
			env: map[string]string{
				"DB_PASSWORD": "pass/word",
			},
			dsn:  "mysql://user:${DB_PASSWORD}@localhost/db",
			want: "mysql://user:pass%2Fword@localhost/db",
		},
		{
			name: "Question mark in password",
			env: map[string]string{
				"DB_PASSWORD": "pass?word",
			},
			dsn:  "mysql://user:${DB_PASSWORD}@localhost/db",
			want: "mysql://user:pass%3Fword@localhost/db",
		},
		{
			name: "Empty password (explicitly set)",
			env: map[string]string{
				"DB_PASSWORD": "",
			},
			dsn:  "mysql://user:${DB_PASSWORD}@localhost/db",
			want: "mysql://user:@localhost/db",
		},
		{
			name: "Space in password",
			env: map[string]string{
				"DB_PASSWORD": "pass word",
			},
			dsn:  "mysql://user:${DB_PASSWORD}@localhost/db",
			want: "mysql://user:pass%20word@localhost/db",
		},
		{
			name: "Newline in password",
			env: map[string]string{
				"DB_PASSWORD": "pass\nword",
			},
			dsn:  "mysql://user:${DB_PASSWORD}@localhost/db",
			want: "mysql://user:pass%0Aword@localhost/db",
		},
		{
			name: "Tab in password",
			env: map[string]string{
				"DB_PASSWORD": "pass\tword",
			},
			dsn:  "mysql://user:${DB_PASSWORD}@localhost/db",
			want: "mysql://user:pass%09word@localhost/db",
		},
		{
			name: "Double quotes in password",
			env: map[string]string{
				"DB_PASSWORD": "pass\"word",
			},
			dsn:  "mysql://user:${DB_PASSWORD}@localhost/db",
			want: "mysql://user:pass%22word@localhost/db",
		},
		{
			name: "Single quote in password",
			env: map[string]string{
				"DB_PASSWORD": "pass'word",
			},
			dsn:  "mysql://user:${DB_PASSWORD}@localhost/db",
			want: "mysql://user:pass%27word@localhost/db",
		},
		{
			name: "Password that looks like numeric sentinel",
			env: map[string]string{
				"DB_PASSWORD": "999000",
			},
			dsn:  "mysql://user:${DB_PASSWORD}@localhost/db",
			want: "mysql://user:999000@localhost/db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got, err := expandDSN(tt.dsn, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("expandDSN() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("expandDSN() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_extractPlaceholders(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		env             map[string]string
		wantSentinel    string
		wantMappingSize int
		wantErr         bool
	}{
		{
			name:            "No placeholders",
			input:           "mysql://user:pass@localhost/db",
			env:             map[string]string{},
			wantSentinel:    "mysql://user:pass@localhost/db",
			wantMappingSize: 0,
		},
		{
			name:  "Single placeholder",
			input: "mysql://user:${PASSWORD}@localhost/db",
			env: map[string]string{
				"PASSWORD": "secret",
			},
			wantSentinel:    "mysql://user:_PH-999000_@localhost/db",
			wantMappingSize: 1,
		},
		{
			name:  "Multiple placeholders",
			input: "mysql://${USER}:${PASSWORD}@${HOST}/db",
			env: map[string]string{
				"USER":     "admin",
				"PASSWORD": "secret",
				"HOST":     "localhost",
			},
			wantSentinel:    "mysql://_PH-999000_:_PH-999001_@_PH-999002_/db",
			wantMappingSize: 3,
		},
		{
			name:  "Same variable twice",
			input: "${VAR}:${VAR}",
			env: map[string]string{
				"VAR": "value",
			},
			wantSentinel:    "_PH-999000_:_PH-999001_",
			wantMappingSize: 2,
		},
		{
			name:  "Port placeholder",
			input: "mysql://user:pass@localhost:${PORT}/db",
			env: map[string]string{
				"PORT": "3306",
			},
			wantSentinel:    "mysql://user:pass@localhost:_PH-999000_/db",
			wantMappingSize: 1,
		},
		{
			name:            "Missing environment variable",
			input:           "mysql://user:${MISSING}@localhost/db",
			env:             map[string]string{},
			wantSentinel:    "",
			wantMappingSize: 0,
			wantErr:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			gotSentinel, gotMapping, err := extractPlaceholders(tt.input, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractPlaceholders() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if gotSentinel != tt.wantSentinel {
				t.Errorf("extractPlaceholders() sentinel = %v, want %v", gotSentinel, tt.wantSentinel)
			}
			if len(gotMapping) != tt.wantMappingSize {
				t.Errorf("extractPlaceholders() mapping size = %v, want %v", len(gotMapping), tt.wantMappingSize)
			}
		})
	}
}

func Test_expandWithMapping(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		mapping map[string]string
		want    string
	}{
		{
			name:    "No sentinels",
			input:   "plain text",
			mapping: map[string]string{},
			want:    "plain text",
		},
		{
			name:  "Single sentinel",
			input: "user:999000",
			mapping: map[string]string{
				"999000": "secret",
			},
			want: "user:secret",
		},
		{
			name:  "Multiple sentinels",
			input: "999000:999001@999002",
			mapping: map[string]string{
				"999000": "admin",
				"999001": "secret",
				"999002": "localhost",
			},
			want: "admin:secret@localhost",
		},
		{
			name:  "Sentinel not in input",
			input: "no sentinels here",
			mapping: map[string]string{
				"999000": "ignored",
			},
			want: "no sentinels here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandWithMapping(tt.input, tt.mapping)
			if got != tt.want {
				t.Errorf("expandWithMapping() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_buildConnectionURL(t *testing.T) {
	t.Setenv("DB_HOST", "db.internal")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "appdb")
	t.Setenv("DB_USER", "app_user")
	t.Setenv("DB_PASSWORD", "s3cr3t!")

	tests := []struct {
		name    string
		opts    ConnectOptions
		want    string
		wantErr bool
	}{
		{
			name: "DSN only",
			opts: ConnectOptions{
				DSN: "postgres://user:pass@localhost:5432/db?sslmode=disable",
			},
			want: "postgres://user:pass@localhost:5432/db?default_query_exec_mode=simple_protocol&sslmode=disable",
		},
		{
			name: "Override DSN components",
			opts: ConnectOptions{
				DSN:      "postgres://user:pass@localhost:5432/db?sslmode=disable",
				Host:     "override.internal",
				Port:     "6543",
				Database: "override_db",
				User:     "override_user",
				Password: "override#pass",
				Params: map[string]string{
					"sslmode":          "require",
					"connect_timeout":  "10",
					"application_name": "baton",
				},
			},
			want: "postgres://override_user:override%23pass@override.internal:6543/override_db?application_name=baton&connect_timeout=10&default_query_exec_mode=simple_protocol&sslmode=require",
		},
		{
			name: "Structured config only",
			opts: ConnectOptions{
				Scheme:   "postgres",
				Host:     "${DB_HOST}",
				Port:     "${DB_PORT}",
				Database: "${DB_NAME}",
				User:     "${DB_USER}",
				Password: "${DB_PASSWORD}",
				Params: map[string]string{
					"sslmode": "disable",
				},
			},
			want: "postgres://app_user:s3cr3t%21@db.internal:5432/appdb?default_query_exec_mode=simple_protocol&sslmode=disable",
		},
		{
			name: "Port without host",
			opts: ConnectOptions{
				Scheme: "postgres",
				Port:   "5432",
			},
			wantErr: true,
		},
		{
			name: "IPv6 host with port",
			opts: ConnectOptions{
				Scheme:   "postgres",
				Host:     "[::1]",
				Port:     "5432",
				Database: "testdb",
				User:     "testuser",
				Password: "testpass",
			},
			want: "postgres://testuser:testpass@[::1]:5432/testdb?default_query_exec_mode=simple_protocol",
		},
		{
			name: "IPv6 host without brackets gets brackets added by JoinHostPort",
			opts: ConnectOptions{
				Scheme:   "postgres",
				Host:     "2001:db8::1",
				Port:     "5432",
				Database: "testdb",
			},
			want: "postgres://[2001:db8::1]:5432/testdb?default_query_exec_mode=simple_protocol",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildConnectionURL(tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.String() != tt.want {
				t.Fatalf("buildConnectionURL() = %s, want %s", got.String(), tt.want)
			}
		})
	}
}

func Test_buildConnectionURL_PostgresSimpleProtocolDefault(t *testing.T) {
	const modeKey = "default_query_exec_mode"

	tests := []struct {
		name      string
		opts      ConnectOptions
		wantMode  string // empty means key must be absent
		wantOther map[string]string
	}{
		{
			name: "postgres DSN without mode injects simple_protocol",
			opts: ConnectOptions{
				DSN: "postgres://user:pass@localhost:5432/db",
			},
			wantMode: "simple_protocol",
		},
		{
			name: "structured postgres with sslmode only also gets simple_protocol",
			opts: ConnectOptions{
				Scheme:   "postgres",
				Host:     "db.example",
				Port:     "5432",
				Database: "app",
				User:     "u",
				Password: "p",
				Params: map[string]string{
					"sslmode": "require",
				},
			},
			wantMode: "simple_protocol",
			wantOther: map[string]string{
				"sslmode": "require",
			},
		},
		{
			name: "DSN already sets cache_statement is preserved",
			opts: ConnectOptions{
				DSN: "postgres://user:pass@localhost:5432/db?default_query_exec_mode=cache_statement",
			},
			wantMode: "cache_statement",
		},
		{
			name: "Params set exec is preserved",
			opts: ConnectOptions{
				Scheme:   "postgres",
				Host:     "db.example",
				Database: "app",
				Params: map[string]string{
					"default_query_exec_mode": "exec",
				},
			},
			wantMode: "exec",
		},
		{
			name: "mysql scheme does not inject mode",
			opts: ConnectOptions{
				Scheme:   "mysql",
				Host:     "db.example",
				Port:     "3306",
				Database: "app",
				User:     "u",
				Password: "p",
			},
			wantMode: "",
		},
		{
			name: "mysql DSN does not inject mode",
			opts: ConnectOptions{
				DSN: "mysql://user:pass@localhost:3306/db",
			},
			wantMode: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildConnectionURL(tt.opts)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			q := got.Query()
			gotMode := q.Get(modeKey)
			if gotMode != tt.wantMode {
				t.Fatalf("%s = %q, want %q (url=%s)", modeKey, gotMode, tt.wantMode, got.String())
			}
			if tt.wantMode == "" {
				if _, ok := q[modeKey]; ok {
					t.Fatalf("%s present on non-postgres url: %s", modeKey, got.String())
				}
			}
			for k, want := range tt.wantOther {
				if got := q.Get(k); got != want {
					t.Fatalf("query %s = %q, want %q", k, got, want)
				}
			}
		})
	}
}
