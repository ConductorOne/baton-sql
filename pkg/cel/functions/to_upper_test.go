package functions

import (
	"testing"

	"github.com/google/cel-go/common/types"
)

func TestToUpper(t *testing.T) {
	type args struct {
		s string
	}
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

func TestToUpperFunc(t *testing.T) {
	funcDef := ToUpperFunc()
	overload := funcDef.Overloads[0]

	input := types.String("test")
	got := overload.Unary(input)
	want := types.String("TEST")

	if got.Equal(want) != types.True {
		t.Errorf("ToUpperFunc() = %v, want %v", got, want)
	}
}
