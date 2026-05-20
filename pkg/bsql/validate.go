package bsql

import (
	"context"
	"errors"
	"fmt"
)

func validateVarsInQuery(s *SQLSyncer, query string, vars map[string]string) error {
	if query == "" {
		return fmt.Errorf("query is required")
	}

	usedVars, err := s.queryVars(query)
	if err != nil {
		return fmt.Errorf("failed to parse query for vars: %w", err)
	}

	if vars == nil {
		vars = make(map[string]string)
	}

	for _, v := range usedVars {
		if _, ok := vars[v]; !ok {
			if v == limitKey || v == offsetKey || v == cursorKey {
				continue
			}
			return fmt.Errorf("query uses variable '%s' which is not defined in vars", v)
		}
	}

	return nil
}

// validateScope rejects typos like `scope: clustr` that would silently downgrade to
// per-database iteration. Empty (default) or "cluster" only.
func validateScope(scope string) error {
	switch scope {
	case "", scopeCluster:
		return nil
	default:
		return fmt.Errorf("invalid scope %q: must be empty or %q", scope, scopeCluster)
	}
}

func (l *ListQuery) staticValidate(ctx context.Context, s *SQLSyncer) error {
	if err := validateScope(l.Scope); err != nil {
		return err
	}
	return validateVarsInQuery(s, l.Query, l.Vars)
}

func (l *EntitlementsQuery) staticValidate(ctx context.Context, s *SQLSyncer) error {
	if err := validateScope(l.Scope); err != nil {
		return err
	}
	for _, mapping := range l.Map {
		if mapping.Provisioning == nil {
			continue
		}

		if mapping.Provisioning.Grant != nil {
			for _, query := range mapping.Provisioning.Grant.Queries {
				err := validateVarsInQuery(s, query, mapping.Provisioning.Vars)
				if err != nil {
					return err
				}
			}
		}

		if mapping.Provisioning.Revoke != nil {
			for _, query := range mapping.Provisioning.Revoke.Queries {
				err := validateVarsInQuery(s, query, mapping.Provisioning.Vars)
				if err != nil {
					return err
				}
			}
		}
	}

	return validateVarsInQuery(s, l.Query, l.Vars)
}

func (l *EntitlementMapping) staticValidate(ctx context.Context, s *SQLSyncer) error {
	if l.Provisioning == nil {
		return nil
	}

	if l.Provisioning.Grant != nil {
		for _, query := range l.Provisioning.Grant.Queries {
			err := validateVarsInQuery(s, query, l.Provisioning.Vars)
			if err != nil {
				return err
			}
		}
	}

	if l.Provisioning.Revoke != nil {
		for _, query := range l.Provisioning.Revoke.Queries {
			err := validateVarsInQuery(s, query, l.Provisioning.Vars)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (l *GrantsQuery) staticValidate(ctx context.Context, s *SQLSyncer) error {
	if err := validateScope(l.Scope); err != nil {
		return err
	}
	return validateVarsInQuery(s, l.Query, l.Vars)
}

func validatePasswordConstraints(constraints []PasswordConstraintConfig) error {
	for i, c := range constraints {
		if c.CharSet == "" {
			return fmt.Errorf("random password constraint[%d]: char_set must not be empty", i)
		}
		if c.MinCount <= 0 {
			return fmt.Errorf("random password constraint[%d]: min_count must be greater than zero", i)
		}
	}
	return nil
}

func (l *AccountProvisioning) staticValidate(ctx context.Context, s *SQLSyncer) error {
	if l.Credentials == nil {
		return errors.New("no credentials defined")
	}

	if l.Credentials.EncryptedPassword == nil &&
		l.Credentials.RandomPassword == nil &&
		l.Credentials.NoPassword == nil {
		return errors.New("no credential method defined")
	}

	if l.Credentials.RandomPassword != nil {
		if l.Credentials.RandomPassword.MaxLength <= 0 {
			return errors.New("random password max_length must be greater than zero")
		}

		if l.Credentials.RandomPassword.MinLength <= 0 {
			return errors.New("random password min_length must be greater than zero")
		}

		if l.Credentials.RandomPassword.MinLength > l.Credentials.RandomPassword.MaxLength {
			return errors.New("random password min_length cannot be greater than max_length")
		}

		if err := validatePasswordConstraints(l.Credentials.RandomPassword.Constraints); err != nil {
			return err
		}
	}

	if l.Create == nil {
		return errors.New("no create functions defined")
	}

	for _, query := range l.Create.Queries {
		err := validateVarsInQuery(s, query, l.Create.Vars)
		if err != nil {
			return err
		}
	}

	if l.Validate == nil {
		return errors.New("no validate functions defined")
	}

	err := validateVarsInQuery(s, l.Validate.Query, l.Validate.Vars)
	if err != nil {
		return err
	}

	return nil
}

func (l *CredentialRotation) staticValidate(ctx context.Context, s *SQLSyncer) error {
	if l.Update != nil {
		for _, query := range l.Update.Queries {
			err := validateVarsInQuery(s, query, l.Update.Vars)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (l *ActionConfig) staticValidate(ctx context.Context, s *SQLSyncer) error {
	availableVars := make(map[string]string)
	for k, v := range l.Vars {
		availableVars[k] = v
	}

	for k, config := range l.Arguments {
		availableVars[k] = config.Type
	}

	return validateVarsInQuery(s, l.Query, availableVars)
}
