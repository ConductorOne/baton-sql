package bsql

import (
	"context"
	"reflect"
	"testing"

	"github.com/conductorone/baton-sql/pkg/database"
)

func Test_clampPageSize(t *testing.T) {
	tests := []struct {
		name           string
		pageSize       int
		configPageSize int
		want           int64
	}{
		{"zero/zero returns default", 0, 0, defaultPageSize},
		{"zero/positive uses config", 0, 25, 25},
		{"zero/negative returns default", 0, -1, defaultPageSize},
		{"zero/config exceeds max clamps to max", 0, 1500, maxPageSize},
		{"zero/config at min boundary", 0, 1, 1},
		{"sdk size takes precedence over config", 50, 25, 50},
		{"sdk size exceeds max clamps to max", 1500, 0, maxPageSize},
		{"sdk size below min clamps to min", -5, 0, minPageSize},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampPageSize(tt.pageSize, tt.configPageSize)
			if got != tt.want {
				t.Errorf("clampPageSize(%d, %d) = %d, want %d", tt.pageSize, tt.configPageSize, got, tt.want)
			}
		})
	}
}

func Test_parseToken(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		want    *queryTokenOpts
		wantErr bool
	}{
		{
			name:  "Basic token without options",
			token: "?<limit>",
			want: &queryTokenOpts{
				Key:      "limit",
				Unquoted: false,
			},
			wantErr: false,
		},
		{
			name:  "Token with unquoted option",
			token: "?<limit|unquoted>",
			want: &queryTokenOpts{
				Key:      "limit",
				Unquoted: true,
			},
			wantErr: false,
		},
		{
			name:  "Token with mixed case",
			token: "?<LIMIT|UNQUOTED>",
			want: &queryTokenOpts{
				Key:      "limit",
				Unquoted: true,
			},
			wantErr: false,
		},
		{
			name:    "Invalid token format",
			token:   "invalid",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Token with unknown option",
			token:   "?<limit|unknown>",
			want:    nil,
			wantErr: true,
		},
		{
			name:  "Token with empty options",
			token: "?<limit|>",
			want: &queryTokenOpts{
				Key:      "limit",
				Unquoted: false,
			},
			wantErr: true,
		},
		{ //nolint:gosec // G101 false positive: "identifier" is the modifier name, not a credential.
			name:  "Token with identifier option",
			token: "?<table_name|identifier>",
			want: &queryTokenOpts{
				Key:        "table_name",
				Identifier: true,
			},
			wantErr: false,
		},
		{ //nolint:gosec // G101 false positive: "identifier" is the modifier name, not a credential.
			name:    "Identifier and unquoted are mutually exclusive",
			token:   "?<x|identifier,unquoted>",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseToken(tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_parseQueryOpts(t *testing.T) {
	type args struct {
		ctx   context.Context
		query string
		pCtx  *paginationContext
		vars  map[string]any
	}
	tests := []struct {
		name           string
		dbEngine       database.DbEngine
		args           args
		query          string
		queryArgs      []interface{}
		paginationUsed bool
		wantErr        bool
	}{
		{
			"Test valid query with no replacements",
			database.MySQL,
			args{
				t.Context(),
				"SELECT * FROM table",
				nil,
				nil,
			},
			"SELECT * FROM table",
			nil,
			false,
			false,
		},
		{
			"Test valid query with same case replacement",
			database.MySQL,
			args{
				t.Context(),
				"SELECT * FROM table LIMIT ?<limit>",
				&paginationContext{
					Limit: 10,
				},
				nil,
			},
			"SELECT * FROM table LIMIT ?",
			[]interface{}{int64(11)},
			true,
			false,
		},
		{
			"Test valid query with different case replacement",
			database.MySQL,
			args{
				t.Context(),
				"SELECT * FROM table LIMIT ?<LIMIT>",
				&paginationContext{
					Limit: 10,
				},
				nil,
			},
			"SELECT * FROM table LIMIT ?",
			[]interface{}{int64(11)},
			true,
			false,
		},
		{
			"Test valid query with multiple replacements (Postgres)",
			database.MySQL,
			args{
				t.Context(),
				"SELECT * FROM table LIMIT ?<LIMIT> OFFSET ?<OFFSET>",
				&paginationContext{
					Limit:  10,
					Offset: 123,
				},
				nil,
			},
			"SELECT * FROM table LIMIT ? OFFSET ?",
			[]interface{}{int64(11), int64(123)},
			true,
			false,
		},
		{
			"Test valid query with multiple replacements (Postgres)",
			database.PostgreSQL,
			args{
				t.Context(),
				"SELECT * FROM table LIMIT ?<LIMIT> OFFSET ?<OFFSET>",
				&paginationContext{
					Limit:  10,
					Offset: 123,
				},
				nil,
			},
			"SELECT * FROM table LIMIT $1 OFFSET $2",
			[]interface{}{int64(11), int64(123)},
			true,
			false,
		},
		{
			"Test valid query with multiple replacements (SQLite)",
			database.SQLite,
			args{
				t.Context(),
				"SELECT * FROM table LIMIT ?<LIMIT> OFFSET ?<OFFSET>",
				&paginationContext{
					Limit:  10,
					Offset: 123,
				},
				nil,
			},
			"SELECT * FROM table LIMIT ? OFFSET ?",
			[]interface{}{int64(11), int64(123)},
			true,
			false,
		},
		{
			"Test valid query with multiple replacements (MSSQL)",
			database.MSSQL,
			args{
				t.Context(),
				"SELECT * FROM table LIMIT ?<LIMIT> OFFSET ?<OFFSET>",
				&paginationContext{
					Limit:  10,
					Offset: 123,
				},
				nil,
			},
			"SELECT * FROM table LIMIT @p1 OFFSET @p2",
			[]interface{}{int64(11), int64(123)},
			true,
			false,
		},
		{
			"Test valid query with multiple replacements (Oracle)",
			database.Oracle,
			args{
				t.Context(),
				"SELECT * FROM table LIMIT ?<LIMIT> OFFSET ?<OFFSET>",
				&paginationContext{
					Limit:  10,
					Offset: 123,
				},
				nil,
			},
			"SELECT * FROM table LIMIT :1 OFFSET :2",
			[]interface{}{int64(11), int64(123)},
			true,
			false,
		},
		{
			"Test valid query with multiple replacements (Vertica)",
			database.Vertica,
			args{
				t.Context(),
				"SELECT * FROM table LIMIT ?<LIMIT> OFFSET ?<OFFSET>",
				&paginationContext{
					Limit:  10,
					Offset: 123,
				},
				nil,
			},
			"SELECT * FROM table LIMIT ? OFFSET ?",
			[]interface{}{int64(11), int64(123)},
			true,
			false,
		},
		{
			"Test valid query with unknown token",
			database.MySQL,
			args{
				t.Context(),
				"SELECT * FROM ?<badToken> LIMIT ?<LIMIT> OFFSET ?<OFFSET>",
				&paginationContext{
					Limit:  10,
					Offset: 0,
				},
				nil,
			},
			"",
			nil,
			false,
			true,
		},
		{
			"Test valid query with unquoted limit",
			database.MySQL,
			args{
				t.Context(),
				"SELECT * FROM table LIMIT ?<limit|unquoted>",
				&paginationContext{
					Limit: 10,
				},
				nil,
			},
			"SELECT * FROM table LIMIT 11",
			nil,
			true,
			false,
		},
		{
			"Test valid query with unquoted offset",
			database.MySQL,
			args{
				t.Context(),
				"SELECT * FROM table OFFSET ?<offset|unquoted>",
				&paginationContext{
					Offset: 123,
				},
				nil,
			},
			"SELECT * FROM table OFFSET 123",
			nil,
			true,
			false,
		},
		{
			"Test valid query with unquoted cursor",
			database.MySQL,
			args{
				t.Context(),
				"SELECT * FROM table WHERE id > ?<cursor|unquoted>",
				&paginationContext{
					Cursor: "abc123",
				},
				nil,
			},
			"SELECT * FROM table WHERE id > abc123",
			nil,
			true,
			false,
		},
		{
			"Test valid query with mixed quoted and unquoted options",
			database.MySQL,
			args{
				t.Context(),
				"SELECT * FROM table WHERE id > ?<cursor> LIMIT ?<limit|unquoted>",
				&paginationContext{
					Cursor: "abc123",
					Limit:  10,
				},
				nil,
			},
			"SELECT * FROM table WHERE id > ? LIMIT 11",
			[]interface{}{"abc123"},
			true,
			false,
		},
		{
			"Test invalid unquoted option",
			database.MySQL,
			args{
				t.Context(),
				"SELECT * FROM table LIMIT ?<limit|invalid>",
				&paginationContext{
					Limit: 10,
				},
				nil,
			},
			"",
			nil,
			false,
			true,
		},
		{
			"Test valid query with var substitution",
			database.MySQL,
			args{
				t.Context(),
				"SELECT * FROM table WHERE test = ?<foo> and answer = ?<bar>",
				nil,
				map[string]any{
					"foo": "test",
					"bar": 42,
				},
			},
			"SELECT * FROM table WHERE test = ? and answer = ?",
			[]interface{}{"test", 42},
			false,
			false,
		},
		{
			"Test valid query with unquoted table name var substitution",
			database.MySQL,
			args{
				t.Context(),
				"SELECT * FROM ?<table_name|unquoted> WHERE test = ?<foo>",
				nil,
				map[string]any{
					"table_name": "example_table",
					"foo":        "test example",
				},
			},
			"SELECT * FROM example_table WHERE test = ?",
			[]interface{}{"test example"},
			false,
			false,
		},
		{
			"Test valid query with identifier substitution (Postgres)",
			database.PostgreSQL,
			args{
				t.Context(),
				"GRANT SELECT ON ?<db|identifier>.?<schema|identifier>.?<tbl|identifier> TO ?<grantee|identifier>",
				nil,
				map[string]any{
					"db":      "analytics",
					"schema":  "public",
					"tbl":     "orders",
					"grantee": "alice",
				},
			},
			`GRANT SELECT ON "analytics"."public"."orders" TO "alice"`,
			nil,
			false,
			false,
		},
		{
			"Test identifier substitution escapes embedded double quote (Postgres)",
			database.PostgreSQL,
			args{
				t.Context(),
				`SELECT * FROM ?<tbl|identifier>`,
				nil,
				map[string]any{
					"tbl": `weird"name`,
				},
			},
			`SELECT * FROM "weird""name"`,
			nil,
			false,
			false,
		},
		{
			"Test identifier substitution uses backticks for MySQL",
			database.MySQL,
			args{
				t.Context(),
				"DROP TABLE ?<tbl|identifier>",
				nil,
				map[string]any{
					"tbl": "my`table",
				},
			},
			"DROP TABLE `my``table`",
			nil,
			false,
			false,
		},
		{
			"Test identifier substitution neutralises injection attempt (Postgres)",
			database.PostgreSQL,
			args{
				t.Context(),
				"GRANT SELECT ON ?<tbl|identifier> TO bob",
				nil,
				map[string]any{
					"tbl": `orders; DROP TABLE users; --`,
				},
			},
			// The semicolons / DROP / comment all become part of one quoted identifier;
			// the parser will reject it as a missing table rather than execute the trailer.
			`GRANT SELECT ON "orders; DROP TABLE users; --" TO bob`,
			nil,
			false,
			false,
		},
		{
			"Test sql injection attempt with unquoted table name var substitution",
			database.MySQL,
			args{
				t.Context(),
				"SELECT * FROM ?<table_name|unquoted> WHERE test = ?<foo>",
				nil,
				map[string]any{
					"table_name": `example_table; DROP TABLE users; --`,
					"foo":        "test example",
				},
			},
			"SELECT * FROM example_tableDROPTABLEusers WHERE test = ?",
			[]interface{}{"test example"},
			false,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ss := &SQLSyncer{
				dbEngine: tt.dbEngine,
			}
			query, queryArgs, paginationUsed, err := ss.parseQueryOpts(tt.args.pCtx, tt.args.query, tt.args.vars)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseQueryOpts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if query != tt.query {
				t.Errorf("parseQueryOpts() got = %v, want %v", query, tt.query)
			}
			if !reflect.DeepEqual(tt.queryArgs, queryArgs) {
				t.Errorf("parseQueryOpts() got = %v, want %v", queryArgs, tt.queryArgs)
			}
			if paginationUsed != tt.paginationUsed {
				t.Errorf("parseQueryOpts() got = %v, want %v", paginationUsed, tt.paginationUsed)
			}
		})
	}
}
