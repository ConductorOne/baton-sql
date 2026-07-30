package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/conductorone/baton-sql/pkg/studio/server"
)

// webFS embeds the Studio UI's static assets (currently a placeholder page;
// Plan 3 replaces cmd/baton-sql-studio/web/index.html with the real UI).
//
//go:embed web
var webFS embed.FS

// runServe implements the "serve" subcommand: it starts the Studio HTTP
// server, hosting both the JSON API (pkg/studio/server) and the static UI
// assets embedded in webFS, and blocks serving until the process exits or
// ListenAndServe returns an error.
func runServe(args []string) error {
	fset := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fset.String("addr", "127.0.0.1:8787", "address to listen on")
	if err := fset.Parse(args); err != nil {
		return err
	}

	staticFS, err := fs.Sub(webFS, "web")
	if err != nil {
		return fmt.Errorf("baton-sql-studio: failed to load embedded web assets: %w", err)
	}

	s := server.New()
	s.SetStatic(staticFS)

	fmt.Printf("baton-sql Studio running at http://%s\n", *addr)
	return http.ListenAndServe(*addr, s.Handler())
}
