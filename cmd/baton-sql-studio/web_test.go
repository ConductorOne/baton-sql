package main

import (
	"os"
	"strings"
	"testing"
)

// TestWeb_IndexHTML asserts the Studio UI shell (Plan 3, Task 1) is present
// and self-contained. It reads the embedded page's source file directly
// rather than spinning up a server, since the markers under test are purely
// structural (HTML/JS text), not server behavior.
func TestWeb_IndexHTML(t *testing.T) {
	data, err := os.ReadFile("web/index.html")
	if err != nil {
		t.Fatalf("failed to read web/index.html: %v", err)
	}
	body := string(data)

	requiredMarkers := []string{
		"baton-sql Studio",
		"Test connection",
		`id="connect"`,
		"/api/connect",
		"/api/status",
		"/api/disconnect",
		// Task 2: resource-type declaration + card rail + workspace tab shell.
		"Declare resource types",
		"What do people get access to?",
		"data-rt-card",
		">List<",
		">Entitlements<",
		">Grants<",
		// Task 3: List tab — query editor, live run, column mapping.
		"/api/run",
		"/api/preview",
		"Map columns",
		"data-map-field",
		"Run",
		"data-map-column",
		"data-map-recipe",
		"data-map-preview",
		// Task 4: Entitlements tab — mode toggle, static list, grantable_to.
		"data-entitlements-mode-toggle",
		"data-entitlements-mode-btn",
		"Static list",
		"From a query",
		"data-entitlements-static",
		"data-entitlements-query",
		"grantable_to",
		"data-grantable-to",
		"data-ent-purpose",
		// Task 5: Grants tab — resource-scoped binding, principal_type,
		// entitlement picker, and the principal_id/skip_if mapping widget.
		"Grants",
		"Who-has-what query",
		"data-grants-query",
		"data-grant-resource-var",
		"data-grant-var-chips",
		"resource.ID",
		"data-grant-principal-type",
		"principal_type",
		"data-grant-entitlement",
		"data-grants-tab",
	}
	for _, marker := range requiredMarkers {
		if !strings.Contains(body, marker) {
			t.Errorf("web/index.html missing required marker %q", marker)
		}
	}

	// The page must be fully self-contained: no CDN-hosted fonts/scripts,
	// no external network calls of any kind. A same-origin relative fetch
	// (e.g. "/api/connect") is fine; an absolute reference to a non-local
	// host is not.
	forbiddenSubstrings := []string{
		"cdn.",
		"googleapis",
		"unpkg",
		"jsdelivr",
	}
	for _, forbidden := range forbiddenSubstrings {
		if strings.Contains(body, forbidden) {
			t.Errorf("web/index.html contains forbidden external-network marker %q", forbidden)
		}
	}

	for _, scheme := range []string{"http://", "https://"} {
		idx := 0
		for {
			i := strings.Index(body[idx:], scheme)
			if i < 0 {
				break
			}
			pos := idx + i
			rest := body[pos+len(scheme):]
			if !strings.HasPrefix(rest, "127.0.0.1") && !strings.HasPrefix(rest, "localhost") {
				t.Errorf("web/index.html references external URL scheme %q at byte %d (not 127.0.0.1/localhost): %q", scheme, pos, rest[:min(40, len(rest))])
			}
			idx = pos + len(scheme)
		}
	}
}
