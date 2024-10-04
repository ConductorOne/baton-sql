package mysql

import "testing"

func Test_convertURItoDSN(t *testing.T) {
	type args struct {
		uri string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			"Test valid URI",
			args{
				"mysql://user:password@localhost:3306/dbname",
			},
			"user:password@tcp(localhost:3306)/dbname",
			false,
		},
		{
			"Postgres URI should fail",
			args{
				"postgres://user:password@localhost:3306/dbname",
			},
			"",
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convertURItoDSN(tt.args.uri)
			if (err != nil) != tt.wantErr {
				t.Errorf("convertURItoDSN() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("convertURItoDSN() got = %v, want %v", got, tt.want)
			}
		})
	}
}
