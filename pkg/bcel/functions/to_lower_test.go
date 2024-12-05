package functions

import (
	"testing"
)

func TestToLower(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"HELLO", "hello"},
		{"", ""},
		{"Hello", "hello"},
		{"H", "h"},
		{"ONE FISH TWO FISH", "one fish two fish"},
	}
	for _, tt := range tests {
		if got := ToLower(tt.input); got != tt.want {
			t.Errorf("ToLower() = %v, want %v", got, tt.want)
		}
	}
}
