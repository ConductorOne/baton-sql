package server

import (
	"encoding/json"
	"net/http"

	"github.com/conductorone/baton-sql/pkg/studio"
)

// generateResponse is the body returned by POST /api/generate. A Generate
// error (e.g. an incomplete Spec) is a normal outcome while a user is
// mid-edit, so it is reported with HTTP 200 and a populated Error rather
// than an HTTP error status.
type generateResponse struct {
	YAML  string `json:"yaml,omitempty"`
	Error string `json:"error,omitempty"`
}

// handleGenerate implements POST /api/generate: it decodes a studio.Spec and
// renders it to connector YAML via studio.Generate. It needs no session
// state (no DB, no CEL env) since Generate only compiles the Spec's own
// field-mapping expressions.
func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, generateResponse{Error: "method not allowed"})
		return
	}

	var spec studio.Spec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeJSON(w, http.StatusOK, generateResponse{Error: "invalid request body: " + err.Error()})
		return
	}

	out, err := studio.Generate(&spec)
	if err != nil {
		writeJSON(w, http.StatusOK, generateResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, generateResponse{YAML: string(out)})
}

// handleValidate implements POST /api/validate: it decodes a studio.Spec and
// authoritatively validates it via studio.Validate, threading the active
// session's DB/engine into ValidateOptions when a session is connected so
// validation runs against the real database rather than the in-memory
// sqlite fallback. A validation failure is reported inside the returned
// Report (OK=false, Errors populated); studio.Validate's error return is
// reserved for unexpected infrastructure failures (e.g. failing to open the
// offline fallback DB), which this handler surfaces as HTTP 500.
func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, studio.Report{Errors: []studio.Issue{{Message: "method not allowed"}}})
		return
	}

	var spec studio.Spec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeJSON(w, http.StatusOK, studio.Report{Errors: []studio.Issue{{Message: "invalid request body: " + err.Error()}}})
		return
	}

	s.mu.Lock()
	db := s.db
	engine := s.engine
	connected := s.connected
	s.mu.Unlock()

	var opts studio.ValidateOptions
	if connected {
		opts = studio.ValidateOptions{DB: db, DBEngine: engine}
	}

	report, err := studio.Validate(r.Context(), &spec, opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, studio.Report{Errors: []studio.Issue{{Message: err.Error()}}})
		return
	}

	writeJSON(w, http.StatusOK, report)
}

// previewRequest is the body of POST /api/preview.
type previewRequest struct {
	Field studio.FieldMapping `json:"field"`
	Row   map[string]any      `json:"row"`
}

// previewResponse is the body returned by POST /api/preview. A PreviewField
// error (e.g. an invalid transform or a row missing a referenced column) is
// a normal outcome while a user is authoring a field mapping, so it is
// reported with HTTP 200 and a populated Error rather than an HTTP error
// status.
type previewResponse struct {
	Value string `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

// handlePreview implements POST /api/preview: it decodes a field mapping and
// sample row and evaluates the mapping's compiled CEL expression against
// that row via studio.PreviewField, using the Server's long-lived CEL
// environment. celEnv is built once in New() and never reassigned, so it is
// read directly without the session mutex.
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, previewResponse{Error: "method not allowed"})
		return
	}

	var req previewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusOK, previewResponse{Error: "invalid request body: " + err.Error()})
		return
	}

	value, err := studio.PreviewField(r.Context(), s.celEnv, req.Field, req.Row)
	if err != nil {
		writeJSON(w, http.StatusOK, previewResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, previewResponse{Value: value})
}
