package functions

import (
	"testing"
)

func TestPHPSerializeArray(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    string
		wantErr bool
	}{
		{
			name:  "administrator",
			input: []string{"administrator"},
			want:  `a:1:{s:13:"administrator";b:1;}`,
		},
		{
			name:  "editor",
			input: []string{"editor"},
			want:  `a:1:{s:6:"editor";b:1;}`,
		},
		{
			name:  "subscriber",
			input: []string{"subscriber"},
			want:  `a:1:{s:10:"subscriber";b:1;}`,
		},
		{
			name:  "contributor",
			input: []string{"contributor"},
			want:  `a:1:{s:11:"contributor";b:1;}`,
		},
		{
			name:  "empty",
			input: []string{},
			want:  `a:0:{}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PHPSerializeStringArray(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("PHPSerializeArray() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("PHPSerializeArray() got = %v, want %v", got, tt.want)
			}
		})
	}
}
