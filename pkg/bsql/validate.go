package bsql

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// queryUsesVar reports whether key appears in the vars slice returned by queryVars.
func queryUsesVar(vars []string, key string) bool {
	for _, v := range vars {
		if v == key {
			return true
		}
	}
	return false
}

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
			if v == limitKey || v == offsetKey || v == cursorKey || v == sinceKey || v == idKey {
				continue
			}
			return fmt.Errorf("query uses variable '%s' which is not defined in vars", v)
		}
	}

	return nil
}

func (l *ListQuery) staticValidate(ctx context.Context, s *SQLSyncer) error {
	return validateVarsInQuery(s, l.Query, l.Vars)
}

func (l *EntitlementsQuery) staticValidate(ctx context.Context, s *SQLSyncer) error {
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
	if err := validateVarsInQuery(s, l.Query, l.Vars); err != nil {
		return err
	}
	if l.IncrementalSync != nil {
		if err := l.IncrementalSync.staticValidate(ctx, s); err != nil {
			return err
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

func (l *GetQuery) staticValidate(ctx context.Context, s *SQLSyncer) error {
	if err := validateVarsInQuery(s, l.Query, l.Vars); err != nil {
		return err
	}
	usedVars, err := s.queryVars(l.Query)
	if err != nil {
		return err
	}
	if !queryUsesVar(usedVars, idKey) {
		return fmt.Errorf("get query must contain ?<%s> placeholder", idKey)
	}
	for k := range l.Vars {
		if k == sinceKey || k == idKey {
			return fmt.Errorf("vars must not use reserved key %q", k)
		}
	}
	return nil
}

func (l *ResourceIncrementalSync) staticValidate(ctx context.Context, s *SQLSyncer) error {
	if l.CursorColumn == "" {
		return errors.New("incremental_sync.cursor_column is required")
	}
	if err := validateVarsInQuery(s, l.Query, l.Vars); err != nil {
		return err
	}
	usedVars, err := s.queryVars(l.Query)
	if err != nil {
		return err
	}
	if !queryUsesVar(usedVars, sinceKey) {
		return fmt.Errorf("incremental_sync.query must contain ?<%s> placeholder", sinceKey)
	}
	for k := range l.Vars {
		if k == sinceKey || k == idKey {
			return fmt.Errorf("vars must not use reserved key %q", k)
		}
	}
	if l.ResourceId != "" {
		if err := s.env.Compile(l.ResourceId); err != nil {
			return fmt.Errorf("incremental_sync.resource_id: invalid CEL expression: %w", err)
		}
	}
	return nil
}

func (l *GrantsIncrementalSync) staticValidate(ctx context.Context, s *SQLSyncer) error {
	if l.ResourceId == "" {
		return errors.New("incremental_sync.resource_id is required")
	}
	if l.ChangesCursorColumn == "" {
		return errors.New("incremental_sync.changes_cursor_column is required")
	}
	if err := validateVarsInQuery(s, l.ChangesQuery, l.Vars); err != nil {
		return fmt.Errorf("incremental_sync.changes_query: %w", err)
	}
	usedVars, err := s.queryVars(l.ChangesQuery)
	if err != nil {
		return err
	}
	if !queryUsesVar(usedVars, sinceKey) {
		return fmt.Errorf("incremental_sync.changes_query must contain ?<%s> placeholder", sinceKey)
	}
	if l.RevokesQuery != "" {
		if l.RevokesCursorColumn == "" {
			return errors.New("incremental_sync.revokes_cursor_column is required when revokes_query is set")
		}
		if err := validateVarsInQuery(s, l.RevokesQuery, l.Vars); err != nil {
			return fmt.Errorf("incremental_sync.revokes_query: %w", err)
		}
		usedVars, err = s.queryVars(l.RevokesQuery)
		if err != nil {
			return err
		}
		if !queryUsesVar(usedVars, sinceKey) {
			return fmt.Errorf("incremental_sync.revokes_query must contain ?<%s> placeholder", sinceKey)
		}
	}
	for k := range l.Vars {
		if k == sinceKey || k == idKey {
			return fmt.Errorf("vars must not use reserved key %q", k)
		}
	}
	return nil
}

func (l *IncrementalSyncConfig) staticValidate(_ context.Context, _ *SQLSyncer) error {
	if l.DefaultLookback != "" {
		if _, err := time.ParseDuration(l.DefaultLookback); err != nil {
			return fmt.Errorf("invalid incremental_sync.default_lookback %q: %w", l.DefaultLookback, err)
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
