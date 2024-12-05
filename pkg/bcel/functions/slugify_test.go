package functions

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		Expr     string
		Expected string
	}{
		{
			Expr:     "Hello, World!",
			Expected: "hello-world",
		},
		{
			Expr:     "GoLang_is Awesome",
			Expected: "golang-is-awesome",
		},
		{
			Expr:     " This--is !a Test ",
			Expected: "this-is-a-test",
		},
		{
			Expr:     "Complex_Example_42",
			Expected: "complex-example-42",
		},
		{
			Expr:     "Multiple   Spaces",
			Expected: "multiple-spaces",
		},
		{
			Expr:     "____leading_and_trailing____",
			Expected: "leading-and-trailing",
		},
		{
			Expr:     "special@#$_characters!!",
			Expected: "special-characters",
		},
		{
			Expr:     "MiXeD CaSe",
			Expected: "mixed-case",
		},
		{
			Expr:     "123 Numbers",
			Expected: "123-numbers",
		},
		{
			Expr:     "Already--slugified",
			Expected: "already-slugified",
		},
	}

	for _, test := range tests {
		t.Run(test.Expr, func(t *testing.T) {
			result := Slugify(test.Expr)
			if result != test.Expected {
				t.Errorf("Slugify(%q) = %q; want %q", test.Expr, result, test.Expected)
			}
		})
	}
}
