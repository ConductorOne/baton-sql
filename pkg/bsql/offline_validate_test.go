package bsql

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func minimalPostgresYAML() string {
	return `
app_name: Golden Postgres
app_description: v1 sync-only fixture
connect:
  scheme: postgres
  host: ${DB_HOST}
  port: ${DB_PORT}
  database: ${DB_DATABASE}
  user: ${DB_USER}
  password: ${DB_PASSWORD}
  params:
    sslmode: disable
resource_types:
  user:
    name: User
    description: Database user
    list:
      query: |
        SELECT id, username, email, status FROM users
      pagination:
        strategy: offset
        primary_key: id
      map:
        id: ".username"
        display_name: ".username"
        description: ".email"
        traits:
          user:
            status: ".status"
            login: ".username"
            emails:
              - ".email"
    static_entitlements:
      - id: member
        display_name: "'Member'"
        description: "'Member of app'"
        purpose: assignment
        grantable_to:
          - user
`
}

func TestOfflineValidate_HappyMinimalPostgres(t *testing.T) {
	cfg, err := Parse([]byte(minimalPostgresYAML()))
	require.NoError(t, err)
	require.NoError(t, OfflineValidate(cfg))
	require.NoError(t, ValidateYAML([]byte(minimalPostgresYAML())))
}

func TestOfflineValidate_NoNetwork(t *testing.T) {
	// Ensure OfflineValidate does not attempt any DB connection for bad configs either.
	err := ValidateYAML([]byte(`
app_name: x
connect:
  scheme: postgres
  host: 127.0.0.1
resource_types:
  user:
    name: User
    list:
      query: SELECT 1
`))
	require.NoError(t, err)
}

func TestRejectNonV1_MultiDB(t *testing.T) {
	cfg, err := Parse([]byte(minimalPostgresYAML()))
	require.NoError(t, err)
	cfg.Connect.Databases = &DatabasesConfig{Static: []string{"a", "b"}}
	err = RejectNonV1ProductFeatures(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "databases")
}

func TestRejectNonV1_Actions(t *testing.T) {
	cfg, err := Parse([]byte(minimalPostgresYAML()))
	require.NoError(t, err)
	cfg.Actions = map[string]ActionConfig{
		"enable": {Name: "Enable", Query: "SELECT 1"},
	}
	err = RejectNonV1ProductFeatures(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "actions")
}

func TestRejectNonV1_AccountProvisioning(t *testing.T) {
	cfg, err := Parse([]byte(minimalPostgresYAML()))
	require.NoError(t, err)
	rt := cfg.ResourceTypes["user"]
	rt.AccountProvisioning = &AccountProvisioning{}
	cfg.ResourceTypes["user"] = rt
	err = RejectNonV1ProductFeatures(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "account_provisioning")
}

func TestRejectNonV1_CredentialRotation(t *testing.T) {
	cfg, err := Parse([]byte(minimalPostgresYAML()))
	require.NoError(t, err)
	rt := cfg.ResourceTypes["user"]
	rt.CredentialRotation = &CredentialRotation{}
	cfg.ResourceTypes["user"] = rt
	err = RejectNonV1ProductFeatures(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "credential_rotation")
}

func TestRejectNonV1_NonPostgresScheme(t *testing.T) {
	cfg, err := Parse([]byte(minimalPostgresYAML()))
	require.NoError(t, err)
	cfg.Connect.Scheme = "mysql"
	err = RejectNonV1ProductFeatures(cfg)
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "postgres")
}

func TestRejectNonV1_PostgresqlAliasRejected(t *testing.T) {
	cfg, err := Parse([]byte(minimalPostgresYAML()))
	require.NoError(t, err)
	cfg.Connect.Scheme = "postgresql"
	err = RejectNonV1ProductFeatures(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "postgresql")
}

func TestRejectNonV1_SchemeFromDSN(t *testing.T) {
	cfg, err := Parse([]byte(`
app_name: DSN App
connect:
  dsn: "postgres://${DB_HOST}:${DB_PORT}/${DB_DATABASE}?sslmode=disable"
  user: "${DB_USER}"
  password: "${DB_PASSWORD}"
resource_types:
  user:
    name: User
    list:
      query: SELECT 1
`))
	require.NoError(t, err)
	require.NoError(t, RejectNonV1ProductFeatures(cfg))
}

func TestOfflineValidate_MissingAppName(t *testing.T) {
	err := ValidateYAML([]byte(`
connect:
  scheme: postgres
  host: localhost
resource_types:
  user:
    name: User
    list:
      query: SELECT 1
`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "app_name")
}
