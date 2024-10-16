package functions

import "testing"

func TestTitleCase(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Basic: lowercase words", "hello world", "Hello World"},
		{"Basic: uppercase words", "HELLO WORLD", "Hello World"},
		{"Basic: mixed case", "hElLo WoRlD", "Hello World"},

		{"Edge: empty string", "", ""},
		{"Edge: single character", "a", "A"},
		{"Edge: two single characters", "a b", "A B"},
		{"Edge: numbers with text", "123 abc", "123 Abc"},

		{"Punctuation: comma and exclamation", "hello, world!", "Hello, World!"},
		{"Punctuation: hyphen", "hello-world!", "Hello-World!"},

		{"Special: numbers inside text", "hello 123 world", "Hello 123 World"},
		{"Special: sentence with numbers", "99 problems but a bug ain't one", "99 Problems But A Bug Ain't One"},

		{"Spaces: multiple spaces between words", "  hello   world  ", "  Hello   World  "},

		{"Mixed case: complex case", "goLang is FUN!", "Golang Is Fun!"},

		{"Unicode: Spanish with punctuation", "¡hola mundo!", "¡Hola Mundo!"},
		{"Unicode: French text", "goé faire un tour", "Goé Faire Un Tour"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TitleCase(tt.input); got != tt.expected {
				t.Errorf("TitleCase() = %v, want %v", got, tt.expected)
			}
		})
	}
}
