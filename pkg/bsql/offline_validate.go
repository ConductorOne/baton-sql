package bsql

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sql/pkg/bcel"
	"github.com/conductorone/baton-sql/pkg/database"
)

// Thrash / illegal parent-bind patterns produced by assist models. Checked offline
// so validate_baton_sql_config fails closed before Apply/sync.
var (
	hybridPlaceholderRegex = regexp.MustCompile(`\$\d+<`)
	dottedPlaceholderRegex = regexp.MustCompile(`\?<[^>\n]*\.[^>\n]*>`)
	bareDollarParamRegex   = regexp.MustCompile(`\$[0-9]+`)
)

// OfflineValidate performs YAML-level structural checks without opening a DB or
// requiring SQLSyncer. Suitable for editor/RPC offline validation.
func OfflineValidate(cfg *Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	if strings.TrimSpace(cfg.AppName) == "" {
		return errors.New("app_name is required")
	}
	if err := validateConnectOffline(&cfg.Connect); err != nil {
		return err
	}
	if len(cfg.ResourceTypes) == 0 {
		return errors.New("resource_types is required")
	}
	for name, rt := range cfg.ResourceTypes {
		if strings.TrimSpace(rt.Name) == "" {
			return fmt.Errorf("resource_types.%s: name is required", name)
		}
		if rt.List == nil || strings.TrimSpace(rt.List.Query) == "" {
			return fmt.Errorf("resource_types.%s: list.query is required", name)
		}
		if err := validateScope(rt.List.Scope); err != nil {
			return fmt.Errorf("resource_types.%s.list: %w", name, err)
		}
	}
	if err := RejectNonV1ProductFeatures(cfg); err != nil {
		return err
	}
	if err := validateQueryPlaceholderLegality(cfg); err != nil {
		return err
	}
	return OfflineValidateGrantBinds(cfg)
}

// validateQueryPlaceholderLegality walks list/entitlements/grants query strings
// for illegal thrash forms. Bare $N hard-fail is grants-only for this slice.
func validateQueryPlaceholderLegality(cfg *Config) error {
	for name, rt := range cfg.ResourceTypes {
		if rt.List != nil {
			if err := checkQueryPlaceholderLegality(
				fmt.Sprintf("resource_types.%s.list.query", name),
				rt.List.Query,
				false,
			); err != nil {
				return err
			}
		}
		if rt.Entitlements != nil {
			if err := checkQueryPlaceholderLegality(
				fmt.Sprintf("resource_types.%s.entitlements.query", name),
				rt.Entitlements.Query,
				false,
			); err != nil {
				return err
			}
		}
		for i, g := range rt.Grants {
			if g == nil {
				continue
			}
			if err := checkQueryPlaceholderLegality(
				fmt.Sprintf("resource_types.%s.grants[%d].query", name, i),
				g.Query,
				true,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkQueryPlaceholderLegality(path, query string, scanBareDollar bool) error {
	if strings.TrimSpace(query) == "" {
		return nil
	}
	// Hybrid before bare: $1<path> also matches \$[0-9]+.
	if hybridPlaceholderRegex.MatchString(query) {
		return fmt.Errorf("%s: forbidden hybrid $N<path> bind; use vars with a simple name and ?<name> (e.g. vars.group_id: \"resource.ID\" and ?<group_id>)", path)
	}
	if dottedPlaceholderRegex.MatchString(query) {
		return fmt.Errorf("%s: placeholder keys may only be [A-Za-z0-9_]; cannot contain '.'; use vars with CEL resource.ID (e.g. vars.group_id: \"resource.ID\" then ?<group_id>)", path)
	}
	if scanBareDollar && bareDollarParamRegex.MatchString(query) {
		return fmt.Errorf("%s: do not use raw $N placeholders in grants queries; use vars + ?<name> and let the engine bind (e.g. vars.group_id: \"resource.ID\" and WHERE col = ?<group_id>)", path)
	}
	return nil
}

// OfflineValidateGrantBinds dry-runs grants ?<...> expansion against a synthetic
// parent resource without opening a database. Unknown tokens or unbound params fail.
func OfflineValidateGrantBinds(cfg *Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	ctx := context.Background()
	env, err := bcel.NewEnv(ctx)
	if err != nil {
		return fmt.Errorf("offline grants bind: cel env: %w", err)
	}
	s := &SQLSyncer{
		dbEngine: database.PostgreSQL,
		env:      env,
	}
	pCtx := &paginationContext{}

	for rtName, rt := range cfg.ResourceTypes {
		for i, g := range rt.Grants {
			if g == nil {
				continue
			}
			path := fmt.Sprintf("resource_types.%s.grants[%d]", rtName, i)
			if err := offlineValidateOneGrantBind(ctx, s, pCtx, rtName, path, g); err != nil {
				return err
			}
		}
	}
	return nil
}

func offlineValidateOneGrantBind(ctx context.Context, s *SQLSyncer, pCtx *paginationContext, rtName, path string, g *GrantsQuery) error {
	tokens, err := s.queryVars(g.Query)
	if err != nil {
		return fmt.Errorf("%s.query: %w", path, err)
	}
	hasNonPagination := false
	for _, tok := range tokens {
		switch tok {
		case limitKey, offsetKey, cursorKey:
			// pagination tokens are engine-supplied
		default:
			hasNonPagination = true
		}
	}
	if !hasNonPagination {
		return nil
	}

	resource := &v2.Resource{
		Id: &v2.ResourceId{
			ResourceType: rtName,
			Resource:     "g2",
		},
		DisplayName: "offline-validate-parent",
	}
	inputs := s.env.SyncInputsWithResource(nil, resource)
	queryVars, err := s.PrepareQueryVars(ctx, inputs, g.Vars)
	if err != nil {
		return fmt.Errorf("%s.vars: %w", path, err)
	}
	// parseToken lowercases placeholder keys; align vars map keys for lookup.
	normalized := make(map[string]any, len(queryVars))
	for k, v := range queryVars {
		normalized[strings.ToLower(k)] = v
	}

	updated, qArgs, _, err := s.parseQueryOpts(pCtx, g.Query, normalized)
	if err != nil {
		return fmt.Errorf("%s.query: %w", path, err)
	}
	_ = updated
	_ = qArgs
	return nil
}

// RejectNonV1ProductFeatures fails configs that are outside the C1 agent-authored
// v1 product surface (Postgres single-DB sync-only).
func RejectNonV1ProductFeatures(cfg *Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	if cfg.Connect.Databases != nil {
		return errors.New("connect.databases is not supported in v1 (single database only)")
	}
	if len(cfg.Actions) > 0 {
		return errors.New("actions are not supported in v1 (sync-only)")
	}
	for name, rt := range cfg.ResourceTypes {
		if rt.AccountProvisioning != nil {
			return fmt.Errorf("resource_types.%s: account_provisioning is not supported in v1", name)
		}
		if rt.CredentialRotation != nil {
			return fmt.Errorf("resource_types.%s: credential_rotation is not supported in v1", name)
		}
	}
	scheme, err := resolveConnectScheme(&cfg.Connect)
	if err != nil {
		return err
	}
	if scheme != "postgres" {
		return fmt.Errorf("scheme %q is not supported in v1; use postgres:// only (not postgresql)", scheme)
	}
	return nil
}

// ValidateYAML parses YAML bytes and runs OfflineValidate.
func ValidateYAML(data []byte) error {
	cfg, err := Parse(data)
	if err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}
	return OfflineValidate(cfg)
}

func validateConnectOffline(c *DatabaseConfig) error {
	if c == nil {
		return errors.New("connect is required")
	}
	hasDSN := strings.TrimSpace(c.DSN) != ""
	hasScheme := strings.TrimSpace(c.Scheme) != ""
	hasHost := strings.TrimSpace(c.Host) != ""
	if !hasDSN && !hasScheme {
		return errors.New("connect: dsn or scheme is required")
	}
	if !hasDSN && hasScheme && !hasHost {
		return errors.New("connect: host is required when using structured fields without dsn")
	}
	return nil
}

func resolveConnectScheme(c *DatabaseConfig) (string, error) {
	if c == nil {
		return "", errors.New("connect is required")
	}
	if s := strings.TrimSpace(c.Scheme); s != "" {
		return strings.ToLower(s), nil
	}
	dsn := strings.TrimSpace(c.DSN)
	if dsn == "" {
		return "", errors.New("connect: scheme or dsn is required")
	}
	// Placeholders like postgres://${HOST}/db — peel scheme before parse when possible.
	if idx := strings.Index(dsn, "://"); idx > 0 {
		return strings.ToLower(dsn[:idx]), nil
	}
	// Entire DSN may be a single ${VAR}; cannot resolve scheme offline without lookup.
	if strings.HasPrefix(dsn, "${") && strings.HasSuffix(dsn, "}") {
		return "", errors.New("connect: scheme must be set explicitly when dsn is a single placeholder")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("connect: invalid dsn: %w", err)
	}
	if u.Scheme == "" {
		return "", errors.New("connect: scheme missing from dsn")
	}
	return strings.ToLower(u.Scheme), nil
}
