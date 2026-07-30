package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/conductorone/baton-sql/pkg/studio"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "compile" {
		fmt.Fprintln(os.Stderr, "usage: baton-sql-studio compile <spec.json>")
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var spec studio.Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		fmt.Fprintln(os.Stderr, "bad spec json:", err)
		os.Exit(1)
	}
	ctx := context.Background()
	rep, err := studio.Validate(ctx, &spec, studio.ValidateOptions{})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out, err := studio.Generate(&spec)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(string(out))
	if !rep.OK {
		for _, is := range rep.Errors {
			fmt.Fprintf(os.Stderr, "! [%s] %s: %s\n", is.Scope, is.Field, is.Message)
		}
		os.Exit(1)
	}
}
