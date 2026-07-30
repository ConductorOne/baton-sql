package server

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/conductorone/baton-sql/pkg/studio"
)

// importRequest is the body of POST /api/import.
type importRequest struct {
	YAML string `json:"yaml"`
}

// importResponse is the body returned by POST /api/import. A bad or
// unparsable YAML document is a normal outcome while a user is loading a
// file, so it is reported with HTTP 200 and a populated Error rather than an
// HTTP error status - the same convention handleGenerate and handlePreview
// use.
type importResponse struct {
	Spec  *studio.Spec `json:"spec,omitempty"`
	Error string       `json:"error,omitempty"`
}

// handleImport implements POST /api/import: it decodes a {yaml: string}
// request body and reverses the YAML into a studio.Spec via
// studio.SpecFromYAML, so the Studio UI can load an existing baton-sql
// connector config for editing. It needs no session state (no DB, no CEL
// env), matching handleGenerate.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, importResponse{Error: "method not allowed"})
		return
	}

	var req importRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err == io.EOF {
			writeJSON(w, http.StatusOK, importResponse{Error: "invalid request body: empty body"})
			return
		}
		writeJSON(w, http.StatusOK, importResponse{Error: "invalid request body: " + err.Error()})
		return
	}

	spec, err := studio.SpecFromYAML([]byte(req.YAML))
	if err != nil {
		writeJSON(w, http.StatusOK, importResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, importResponse{Spec: spec})
}
