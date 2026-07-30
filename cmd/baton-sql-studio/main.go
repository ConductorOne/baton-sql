package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/conductorone/baton-sql/pkg/studio"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "compile":
		runCompile(os.Args[2:])
	case "serve":
		if err := runServe(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(2)
	}
}

// usage prints the top-level command listing for baton-sql-studio.
func usage() {
	fmt.Fprintln(os.Stderr, "usage: baton-sql-studio compile <spec.json>")
	fmt.Fprintln(os.Stderr, "       baton-sql-studio serve [-addr host:port]")
}

// runCompile implements the "compile" subcommand: unchanged behavior from
// before the "serve" subcommand was added.
func runCompile(args []string) {
	if len(args) < 1 {
		usage()
		os.Exit(2)
	}
	data, err := os.ReadFile(args[0])
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
