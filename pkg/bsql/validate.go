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
			if v == limitKey || v == offsetKey || v == cursorKey || v == sinceKey || v == resourceIdKey {
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

		if err := validateGrantProvisioningQueries(s, mapping.Provisioning.Grant, mapping.Provisioning.Vars); err != nil {
			return err
		}

		if err := validateRevokeProvisioningQueries(s, mapping.Provisioning.Revoke, mapping.Provisioning.Vars); err != nil {
			return err
		}
	}

	return validateVarsInQuery(s, l.Query, l.Vars)
}

func (l *EntitlementMapping) staticValidate(ctx context.Context, s *SQLSyncer) error {
	if l.Provisioning == nil {
		return nil
	}

	if err := validateGrantProvisioningQueries(s, l.Provisioning.Grant, l.Provisioning.Vars); err != nil {
		return err
	}

	if err := validateRevokeProvisioningQueries(s, l.Provisioning.Revoke, l.Provisioning.Vars); err != nil {
		return err
	}

	return nil
}

// validateProvisioningIdempotency validates the validation_queries and the
// validation_queries_signal_idempotency opt-in shared by grant and revoke. Validation queries are
// always vars-checked. The opt-in is only valid on DDL engines (Db2, Oracle) and requires at least
// one validation query. no_transaction is required whenever a no-rows validation is reinterpreted
// as idempotent (Db2 by default, Oracle on opt-in): Db2/Oracle GRANT/REVOKE report no rows-affected
// under a tx, which would roll the statement back.
func validateProvisioningIdempotency(s *SQLSyncer, pq EntitlementProvisioningQueries, vars map[string]string) error {
	for _, query := range pq.ValidationQueries {
		if err := validateVarsInQuery(s, query, vars); err != nil {
			return err
		}
	}

	if pq.ValidationQueriesSignalIdempotency {
		if !isDDLEngine(s.dbEngine) {
			return errors.New("validation_queries_signal_idempotency is only supported on DDL engines (Db2, Oracle)")
		}
		if len(pq.ValidationQueries) < 1 {
			return errors.New("validation_queries_signal_idempotency requires at least one validation_query")
		}
	}

	if len(pq.ValidationQueries) > 0 &&
		s.validationNoRowsMeansIdempotent(pq.ValidationQueriesSignalIdempotency) &&
		!pq.NoTransaction {
		return errors.New("validation_queries with no-rows idempotency require no_transaction: true on DDL engines (Db2/Oracle)")
	}

	return nil
}

func validateGrantProvisioningQueries(s *SQLSyncer, grant *GrantEntitlementProvisioningQueries, vars map[string]string) error {
	if grant == nil {
		return nil
	}

	if grant.RejectIf != nil {
		if err := validateVarsInQuery(s, grant.RejectIf.Query, vars); err != nil {
			return err
		}
	}

	if err := validateProvisioningIdempotency(s, grant.EntitlementProvisioningQueries, vars); err != nil {
		return err
	}

	for _, query := range grant.Queries {
		if err := validateVarsInQuery(s, query, vars); err != nil {
			return err
		}
	}

	return nil
}

func validateRevokeProvisioningQueries(s *SQLSyncer, revoke *RevokeEntitlementProvisioningQueries, vars map[string]string) error {
	if revoke == nil {
		return nil
	}

	if err := validateProvisioningIdempotency(s, revoke.EntitlementProvisioningQueries, vars); err != nil {
		return err
	}

	for _, query := range revoke.Queries {
		if err := validateVarsInQuery(s, query, vars); err != nil {
			return err
		}
	}

	if revoke.RevokeOptions != nil && revoke.RevokeOptions.PrincipalExistsCheck != nil {
		if revoke.RevokeOptions.PrincipalExistsCheck.Query == "" {
			return errors.New("principal_exists_check requires a non-empty query")
		}
		if err := validateVarsInQuery(s, revoke.RevokeOptions.PrincipalExistsCheck.Query, vars); err != nil {
			return err
		}
	}

	return nil
}

func (l *GrantsQuery) staticValidate(ctx context.Context, s *SQLSyncer) error {
	if err := validateScope(l.Scope); err != nil {
		return err
	}
	if err := validateVarsInQuery(s, l.Query, l.Vars); err != nil {
		return err
	}
	if l.EventFeed != nil {
		if err := l.EventFeed.staticValidate(ctx, s); err != nil {
			return err
		}
	}
	return nil
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

func (l *GetQuery) staticValidate(ctx context.Context, s *SQLSyncer) error {
	if err := validateVarsInQuery(s, l.Query, l.Vars); err != nil {
		return err
	}
	usedVars, err := s.queryVars(l.Query)
	if err != nil {
		return err
	}
	if !queryUsesVar(usedVars, resourceIdKey) {
		return fmt.Errorf("get query must contain ?<%s> placeholder", resourceIdKey)
	}
	for k := range l.Vars {
		if k == sinceKey || k == resourceIdKey {
			return fmt.Errorf("vars must not use reserved key %q", k)
		}
	}
	return nil
}

// validateEventFeedPagination requires offset pagination on event feed queries so each page stays bounded.
// Cursor paging resumes from the last primary key, which skips rows when results are ordered by timestamp.
func validateEventFeedPagination(field string, p *Pagination, usedVars []string) error {
	if p == nil {
		return fmt.Errorf("%s.pagination is required", field)
	}
	if p.Strategy != offsetKey {
		return fmt.Errorf("%s.pagination.strategy must be %q for event feed queries; %q cannot resume a timestamp-ordered query", field, offsetKey, cursorKey)
	}
	if p.PrimaryKey == "" {
		return fmt.Errorf("%s.pagination.primary_key is required", field)
	}
	if !queryUsesVar(usedVars, limitKey) {
		return fmt.Errorf("%s must contain ?<Limit> placeholder", field)
	}
	return nil
}

func (l *ResourceEventFeed) staticValidate(ctx context.Context, s *SQLSyncer) error {
	if l.CursorColumn == "" {
		return errors.New("event_feed.cursor_column is required")
	}
	if err := validateVarsInQuery(s, l.Query, l.Vars); err != nil {
		return err
	}
	usedVars, err := s.queryVars(l.Query)
	if err != nil {
		return err
	}
	if !queryUsesVar(usedVars, sinceKey) {
		return fmt.Errorf("event_feed.query must contain ?<%s> placeholder", sinceKey)
	}
	if err := validateEventFeedPagination("event_feed.query", l.Pagination, usedVars); err != nil {
		return err
	}
	for k := range l.Vars {
		if k == sinceKey || k == resourceIdKey {
			return fmt.Errorf("vars must not use reserved key %q", k)
		}
	}
	if l.ResourceId != "" {
		if err := s.env.Compile(l.ResourceId); err != nil {
			return fmt.Errorf("event_feed.resource_id: invalid CEL expression: %w", err)
		}
	}
	return nil
}

func (l *GrantsEventFeed) staticValidate(ctx context.Context, s *SQLSyncer) error {
	if l.ResourceId == "" {
		return errors.New("event_feed.resource_id is required")
	}
	if err := s.env.Compile(l.ResourceId); err != nil {
		return fmt.Errorf("event_feed.resource_id: invalid CEL expression: %w", err)
	}
	if l.ChangesCursorColumn == "" {
		return errors.New("event_feed.changes_cursor_column is required")
	}
	if err := validateVarsInQuery(s, l.ChangesQuery, l.Vars); err != nil {
		return fmt.Errorf("event_feed.changes_query: %w", err)
	}
	usedVars, err := s.queryVars(l.ChangesQuery)
	if err != nil {
		return err
	}
	if !queryUsesVar(usedVars, sinceKey) {
		return fmt.Errorf("event_feed.changes_query must contain ?<%s> placeholder", sinceKey)
	}
	if err := validateEventFeedPagination("event_feed.changes_query", l.Pagination, usedVars); err != nil {
		return err
	}
	if l.RevokesQuery != "" {
		if l.RevokesCursorColumn == "" {
			return errors.New("event_feed.revokes_cursor_column is required when revokes_query is set")
		}
		if err := validateVarsInQuery(s, l.RevokesQuery, l.Vars); err != nil {
			return fmt.Errorf("event_feed.revokes_query: %w", err)
		}
		usedVars, err = s.queryVars(l.RevokesQuery)
		if err != nil {
			return err
		}
		if !queryUsesVar(usedVars, sinceKey) {
			return fmt.Errorf("event_feed.revokes_query must contain ?<%s> placeholder", sinceKey)
		}
		if err := validateEventFeedPagination("event_feed.revokes_query", l.Pagination, usedVars); err != nil {
			return err
		}
	}
	for k := range l.Vars {
		if k == sinceKey || k == resourceIdKey {
			return fmt.Errorf("vars must not use reserved key %q", k)
		}
	}
	return nil
}

func (l *EventFeedConfig) staticValidate(_ context.Context, _ *SQLSyncer) error {
	if l.DefaultLookback != "" {
		if _, err := time.ParseDuration(l.DefaultLookback); err != nil {
			return fmt.Errorf("invalid event_feed.default_lookback %q: %w", l.DefaultLookback, err)
		}
	}
	if l.Overlap != "" {
		d, err := time.ParseDuration(l.Overlap)
		if err != nil {
			return fmt.Errorf("invalid event_feed.overlap %q: %w", l.Overlap, err)
		}
		if d < 0 {
			return fmt.Errorf("invalid event_feed.overlap %q: must not be negative", l.Overlap)
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

	// An action defines either `query` or `queries`; both forms are accepted by
	// ActionConfig.Validate and by the action handler, so validate whichever is set.
	if len(l.Queries) > 0 {
		for _, query := range l.Queries {
			if err := validateVarsInQuery(s, query, availableVars); err != nil {
				return err
			}
		}
		return nil
	}

	return validateVarsInQuery(s, l.Query, availableVars)
}
