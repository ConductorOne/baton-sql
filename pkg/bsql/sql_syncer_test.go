package bsql

import (
	"context"
	"testing"
)

func Test_parseQueryOpts(t *testing.T) {
	type args struct {
		ctx    context.Context
		query  string
		values map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			"Test valid query with no replacements",
			args{
				context.Background(),
				"SELECT * FROM table",
				nil,
			},
			"SELECT * FROM table",
			false,
		},
		{
			"Test valid query with same case replacement",
			args{
				context.Background(),
				"SELECT * FROM table LIMIT ?<limit>",
				map[string]string{
					"limit": "10",
				},
			},
			"SELECT * FROM table LIMIT 10",
			false,
		},
		{
			"Test valid query with different case replacement",
			args{
				context.Background(),
				"SELECT * FROM table LIMIT ?<LIMIT>",
				map[string]string{
					"limit": "10",
				},
			},
			"SELECT * FROM table LIMIT 10",
			false,
		},
		{
			"Test valid query with missing replacement",
			args{
				context.Background(),
				"SELECT * FROM table LIMIT ?<LIMIT> OFFSET ?<OFFSET>",
				map[string]string{
					"limit": "10",
				},
			},
			"",
			true,
		},
		{
			"Test valid query with unused replacement",
			args{
				context.Background(),
				"SELECT * FROM table LIMIT ?<LIMIT>",
				map[string]string{
					"limit":  "10",
					"offset": "20",
				},
			},
			"SELECT * FROM table LIMIT 10",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseQueryOpts(tt.args.ctx, tt.args.query, tt.args.values)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseQueryOpts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseQueryOpts() got = %v, want %v", got, tt.want)
			}
		})
	}
}
