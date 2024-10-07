package functions

import (
	"testing"
)

func TestToUpper(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "HELLO"},
		{"", ""},
		{"Hello", "HELLO"},
		{"h", "H"},
		{"one fish two fish", "ONE FISH TWO FISH"},
	}
	for _, tt := range tests {
		if got := ToUpper(tt.input); got != tt.want {
			t.Errorf("ToUpper() = %v, want %v", got, tt.want)
		}
	}
}
