package connector

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewFromYAML_LookupWithoutEnv(t *testing.T) {
	// Use an unsupported scheme so Connect fails before dialing — we only assert
	// that placeholders resolved via LookupFunc (error names resolved host, not ${}).
	yaml := []byte(`
app_name: Test
connect:
  scheme: unsupported-engine
  host: ${DB_HOST}
  database: ${DB_DATABASE}
  user: ${DB_USER}
  password: ${DB_PASSWORD}
resource_types:
  user:
    name: User
    list:
      query: SELECT 1
`)
	lookup := func(key string) (string, bool) {
		m := map[string]string{
			"DB_HOST":     "lookup-host.example",
			"DB_DATABASE": "lookup-db",
			"DB_USER":     "lookup-user",
			"DB_PASSWORD": "lookup-pass-secret",
		}
		v, ok := m[key]
		return v, ok
	}

	// Clear env so resolution cannot come from process environment.
	t.Setenv("DB_HOST", "")
	t.Setenv("DB_DATABASE", "")
	t.Setenv("DB_USER", "")
	t.Setenv("DB_PASSWORD", "")

	_, err := NewFromYAML(t.Context(), yaml, lookup)
	require.Error(t, err)
	// Expansion succeeded; connect failed on scheme after building URL.
	require.Contains(t, err.Error(), "unsupported database scheme")
	// Must not leave unresolved placeholders in the error path for scheme.
	require.False(t, strings.Contains(err.Error(), "${DB_HOST}"))
}

func TestNewFromYAML_MissingLookupKey(t *testing.T) {
	yaml := []byte(`
app_name: Test
connect:
  scheme: postgres
  host: ${MISSING_KEY}
  database: db
  user: u
  password: p
resource_types:
  user:
    name: User
    list:
      query: SELECT 1
`)
	lookup := func(key string) (string, bool) {
		return "", false
	}
	_, err := NewFromYAML(t.Context(), yaml, lookup)
	require.Error(t, err)
	require.Contains(t, err.Error(), "MISSING_KEY")
}

func TestNewWithConfig_Nil(t *testing.T) {
	_, err := NewWithConfig(t.Context(), nil)
	require.Error(t, err)
}
