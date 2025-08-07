package functions

import (
	"reflect"
	"testing"
)

func TestPHPDeserializeArray(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		want    []string
		wantErr bool
	}{
		{
			name: "administrator_role",
			expr: `a:1:{s:13:"administrator";b:1;}`,
			want: []string{"administrator"},
		},
		{
			name: "editor_role",
			expr: `a:1:{s:6:"editor";b:1;}`,
			want: []string{"editor"},
		},
		{
			name: "subscriber_role",
			expr: `a:1:{s:10:"subscriber";b:1;}`,
			want: []string{"subscriber"},
		},
		{
			name: "contributor_role",
			expr: `a:1:{s:11:"contributor";b:1;}`,
			want: []string{"contributor"},
		},
		{
			name: "empty",
			expr: `a:0:{}`,
			want: []string{},
		},
		{
			name:    "invalid",
			expr:    `foobar`,
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PHPDeserializeStringArray(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("PHPDeserializeArray() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PHPDeserializeArray() got = %v, want %v", got, tt.want)
			}
		})
	}
}
