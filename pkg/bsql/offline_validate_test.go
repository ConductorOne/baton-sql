package bsql

import (
	"strings"
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sql/pkg/bcel"
	"github.com/conductorone/baton-sql/pkg/database"
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

func withGroupResourceAndGrants(grantsBlock string) string {
	return `
app_name: Golden Postgres
app_description: grants thrash fixture
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
    list:
      query: |
        SELECT id FROM public.c1_smoke_users
      map:
        id: ".id"
        display_name: ".id"
  group:
    name: Group
    list:
      query: |
        SELECT id FROM public.c1_smoke_groups
      map:
        id: ".id"
        display_name: ".id"
    static_entitlements:
      - id: member
        display_name: "'Member'"
        purpose: assignment
        grantable_to:
          - user
` + grantsBlock
}

func TestOfflineValidate_RejectsBareDollarGrant(t *testing.T) {
	yaml := withGroupResourceAndGrants(`
    grants:
      - query: |
          SELECT user_id FROM public.c1_smoke_group_members WHERE group_id = $1
        map:
          - principal_id: ".user_id"
            principal_type: user
            entitlement_id: member
`)
	err := ValidateYAML([]byte(yaml))
	require.Error(t, err)
	require.True(t,
		strings.Contains(err.Error(), "raw $") || strings.Contains(err.Error(), "do not use raw"),
		"error=%v", err)
}

func TestOfflineValidate_RejectsDottedPlaceholder(t *testing.T) {
	yaml := withGroupResourceAndGrants(`
    grants:
      - query: |
          SELECT user_id FROM public.c1_smoke_group_members WHERE group_id = ?<ResourceID.Resource>
        map:
          - principal_id: ".user_id"
            principal_type: user
            entitlement_id: member
`)
	err := ValidateYAML([]byte(yaml))
	require.Error(t, err)
	require.True(t,
		strings.Contains(err.Error(), "placeholder keys may only") || strings.Contains(err.Error(), "cannot contain '.'"),
		"error=%v", err)
}

func TestOfflineValidate_RejectsHybridDollarPath(t *testing.T) {
	yaml := withGroupResourceAndGrants(`
    grants:
      - query: |
          SELECT user_id FROM public.c1_smoke_group_members WHERE group_id = $1<ResourceID.Resource>
        map:
          - principal_id: ".user_id"
            principal_type: user
            entitlement_id: member
`)
	err := ValidateYAML([]byte(yaml))
	require.Error(t, err)
	require.True(t,
		strings.Contains(err.Error(), "$N<path>") || strings.Contains(strings.ToLower(err.Error()), "hybrid"),
		"error=%v", err)
}

func TestOfflineValidate_AcceptsD1MembershipGrants(t *testing.T) {
	yaml := withGroupResourceAndGrants(`
    grants:
      - vars:
          group_id: "resource.ID"
        query: |
          SELECT gm.user_id
          FROM public.c1_smoke_group_members gm
          WHERE gm.group_id = ?<group_id>
          ORDER BY gm.user_id
        map:
          - principal_id: ".user_id"
            principal_type: user
            entitlement_id: member
`)
	require.NoError(t, ValidateYAML([]byte(yaml)))

	cfg, err := Parse([]byte(yaml))
	require.NoError(t, err)
	// Dry-run assert: postgres rewrite binds synthetic parent id g2.
	ctx := t.Context()
	env, err := bcel.NewEnv(ctx)
	require.NoError(t, err)
	s := &SQLSyncer{dbEngine: database.PostgreSQL, env: env}
	g := cfg.ResourceTypes["group"].Grants[0]
	resource := &v2.Resource{
		Id: &v2.ResourceId{ResourceType: "group", Resource: "g2"},
	}
	inputs := s.env.SyncInputsWithResource(nil, resource)
	queryVars, err := s.PrepareQueryVars(ctx, inputs, g.Vars)
	require.NoError(t, err)
	normalized := map[string]any{}
	for k, v := range queryVars {
		normalized[strings.ToLower(k)] = v
	}
	updated, qArgs, _, err := s.parseQueryOpts(&paginationContext{}, g.Query, normalized)
	require.NoError(t, err)
	require.Contains(t, updated, "$1")
	require.Len(t, qArgs, 1)
	require.Equal(t, "g2", qArgs[0])
}

func TestOfflineValidate_AcceptsD2SkipIfOnly(t *testing.T) {
	yaml := withGroupResourceAndGrants(`
    grants:
      - query: |
          SELECT group_id, user_id FROM public.c1_smoke_group_members
        map:
          - skip_if: "cols.group_id != resource.ID"
            principal_id: ".user_id"
            principal_type: user
            entitlement_id: member
`)
	require.NoError(t, ValidateYAML([]byte(yaml)))
}

func TestOfflineValidate_UnknownGrantToken(t *testing.T) {
	yaml := withGroupResourceAndGrants(`
    grants:
      - query: |
          SELECT user_id FROM public.c1_smoke_group_members WHERE group_id = ?<group_id>
        map:
          - principal_id: ".user_id"
            principal_type: user
            entitlement_id: member
`)
	err := ValidateYAML([]byte(yaml))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown token")
}
