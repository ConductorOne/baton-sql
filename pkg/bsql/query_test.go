package bsql

import (
	"context"
	"reflect"
	"testing"

	"github.com/conductorone/baton-sql/pkg/database"
)

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
				context.Background(),
				"SELECT * FROM table",
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
				context.Background(),
				"SELECT * FROM table LIMIT ?<limit>",
				&paginationContext{
					Limit: 10,
				},
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
				context.Background(),
				"SELECT * FROM table LIMIT ?<LIMIT>",
				&paginationContext{
					Limit: 10,
				},
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
				context.Background(),
				"SELECT * FROM table LIMIT ?<LIMIT> OFFSET ?<OFFSET>",
				&paginationContext{
					Limit:  10,
					Offset: 123,
				},
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
				context.Background(),
				"SELECT * FROM table LIMIT ?<LIMIT> OFFSET ?<OFFSET>",
				&paginationContext{
					Limit:  10,
					Offset: 123,
				},
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
				context.Background(),
				"SELECT * FROM table LIMIT ?<LIMIT> OFFSET ?<OFFSET>",
				&paginationContext{
					Limit:  10,
					Offset: 123,
				},
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
				context.Background(),
				"SELECT * FROM table LIMIT ?<LIMIT> OFFSET ?<OFFSET>",
				&paginationContext{
					Limit:  10,
					Offset: 123,
				},
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
				context.Background(),
				"SELECT * FROM table LIMIT ?<LIMIT> OFFSET ?<OFFSET>",
				&paginationContext{
					Limit:  10,
					Offset: 123,
				},
			},
			"SELECT * FROM table LIMIT :1 OFFSET :2",
			[]interface{}{int64(11), int64(123)},
			true,
			false,
		},
		{
			"Test valid query with unknown token",
			database.MySQL,
			args{
				context.Background(),
				"SELECT * FROM ?<badToken> LIMIT ?<LIMIT> OFFSET ?<OFFSET>",
				&paginationContext{
					Limit:  10,
					Offset: 0,
				},
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
				context.Background(),
				"SELECT * FROM table LIMIT ?<limit|unquoted>",
				&paginationContext{
					Limit: 10,
				},
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
				context.Background(),
				"SELECT * FROM table OFFSET ?<offset|unquoted>",
				&paginationContext{
					Offset: 123,
				},
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
				context.Background(),
				"SELECT * FROM table WHERE id > ?<cursor|unquoted>",
				&paginationContext{
					Cursor: "abc123",
				},
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
				context.Background(),
				"SELECT * FROM table WHERE id > ?<cursor> LIMIT ?<limit|unquoted>",
				&paginationContext{
					Cursor: "abc123",
					Limit:  10,
				},
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
				context.Background(),
				"SELECT * FROM table LIMIT ?<limit|invalid>",
				&paginationContext{
					Limit: 10,
				},
			},
			"",
			nil,
			false,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ss := &SQLSyncer{
				dbEngine: tt.dbEngine,
			}
			query, queryArgs, paginationUsed, err := ss.parseQueryOpts(tt.args.ctx, tt.args.pCtx, tt.args.query)
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
