package bsql

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
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
	return RejectNonV1ProductFeatures(cfg)
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
