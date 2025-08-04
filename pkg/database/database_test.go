package database

import (
	"context"
	"os"
	"testing"
)

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
