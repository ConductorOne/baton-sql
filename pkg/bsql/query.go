package bsql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	sdkGrant "github.com/conductorone/baton-sdk/pkg/types/grant"
	"github.com/conductorone/baton-sql/pkg/helpers"
	"github.com/google/cel-go/common/types"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"

	"github.com/conductorone/baton-sdk/pkg/pagination"
	"github.com/conductorone/baton-sql/pkg/database"
)

const (
	maxPageSize     = 1000
	minPageSize     = 1
	defaultPageSize = 100
	offsetKey       = "offset"
	cursorKey       = "cursor"
	limitKey        = "limit"
	unquotedKey     = "unquoted"
	identifierKey   = "identifier"
)

var ErrQueryAffectedZeroRows = errors.New("query affected 0 rows, ending and rolling back")
var ErrQueryAffectedMoreThanOneRow = errors.New("query affected more than one row, ending and rolling back")

const defaultGrantCancelledReason = "Grant cancelled by connector policy."

type executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

var identSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_]+`)

func SanitizeIdentifier(s string) string {
	return identSanitizer.ReplaceAllString(s, "")
}

type paginationContext struct {
	Strategy   string
	Limit      int64
	Offset     int64
	Cursor     string
	PrimaryKey string
}

type queryTokenOpts struct {
	Key string

	// Unquoted strips non-alphanumeric chars and inlines as-is. Drops, never escapes —
	// don't use with untrusted input.
	Unquoted bool

	// Identifier inlines as an engine-quoted SQL identifier (doubled embedded quotes).
	// Use where parameter binding isn't allowed by the SQL grammar (GRANT, DDL).
	Identifier bool
}

var queryOptRegex = regexp.MustCompile(`\?\<([a-zA-Z0-9_]+)(?:\|([a-zA-Z0-9_]+))?\>`)

func (s *SQLSyncer) getNextPlaceholder(qArgs []interface{}) string {
	switch s.dbEngine {
	case database.MySQL:
		return "?"
	case database.PostgreSQL:
		return fmt.Sprintf("$%d", len(qArgs))
	case database.SQLite:
		return "?"
	case database.MSSQL:
		return fmt.Sprintf("@p%d", len(qArgs))
	case database.Oracle:
		return fmt.Sprintf(":%d", len(qArgs))
	case database.Vertica:
		return "?"
	case database.DB2:
		return "?"
	default:
		return "?"
	}
}

func parseToken(token string) (*queryTokenOpts, error) {
	matches := queryOptRegex.FindStringSubmatch(token)
	if len(matches) == 0 {
		return nil, fmt.Errorf("invalid token format: %s", token)
	}

	key := strings.ToLower(matches[1])
	opts := &queryTokenOpts{
		Key: key,
	}

	if len(matches) < 3 {
		return opts, nil
	}

	optStr := strings.ToLower(matches[2])
	if optStr == "" {
		return opts, nil
	}

	for _, opt := range strings.Split(optStr, ",") {
		opt = strings.TrimSpace(strings.ToLower(opt))
		switch opt {
		case unquotedKey:
			opts.Unquoted = true
		case identifierKey:
			opts.Identifier = true
		default:
			return nil, fmt.Errorf("unknown option %s", opt)
		}
	}

	if opts.Unquoted && opts.Identifier {
		return nil, fmt.Errorf("token options unquoted and identifier are mutually exclusive")
	}

	return opts, nil
}

func (s *SQLSyncer) quoteIdentifier(name string) string {
	if s.dbEngine == database.MySQL {
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (s *SQLSyncer) queryVars(query string) ([]string, error) {
	result := make([]string, 0)

	for _, token := range queryOptRegex.FindAllString(query, -1) {
		opts, err := parseToken(token)
		if err != nil {
			return nil, err
		}
		result = append(result, opts.Key)
	}

	return result, nil
}

func (s *SQLSyncer) parseQueryOpts(pCtx *paginationContext, query string, vars map[string]any) (string, []interface{}, bool, error) {
	if vars == nil {
		vars = make(map[string]any)
	}

	var qArgs []interface{}

	var parseErr error
	paginationOptSet := false
	updatedQuery := queryOptRegex.ReplaceAllStringFunc(query, func(token string) string {
		opts, err := parseToken(token)
		if err != nil {
			parseErr = errors.Join(parseErr, fmt.Errorf("in token %s: %w", token, err))
			return token
		}

		var val interface{}
		switch opts.Key {
		case limitKey:
			// Always request 1 more than the specified limit, so we can see if there are additional results.
			val = pCtx.Limit + 1
			paginationOptSet = true
		case offsetKey:
			val = pCtx.Offset
			paginationOptSet = true
		case cursorKey:
			val = pCtx.Cursor
			paginationOptSet = true
		default:
			v, ok := vars[opts.Key]
			if !ok {
				parseErr = errors.Join(parseErr, fmt.Errorf("unknown token %s", token))
				return token
			}

			val = v
		}

		// If the value is unquoted, directly insert the value as a string
		if opts.Unquoted {
			return SanitizeIdentifier(fmt.Sprintf("%v", val))
		}

		if opts.Identifier {
			return s.quoteIdentifier(fmt.Sprintf("%v", val))
		}

		qArgs = append(qArgs, val)
		return s.getNextPlaceholder(qArgs)
	})
	if parseErr != nil {
		return "", nil, false, parseErr
	}
	return updatedQuery, qArgs, paginationOptSet, nil
}

func clampPageSize(pageSize int, configPageSize int) int64 {
	if pageSize == 0 {
		if configPageSize > 0 {
			pageSize = configPageSize
		} else {
			return defaultPageSize
		}
	}

	if pageSize > maxPageSize {
		return maxPageSize
	}
	if pageSize < minPageSize {
		return minPageSize
	}
	return int64(pageSize)
}

func (s *SQLSyncer) prepareQuery(pToken *pagination.Token, query string, pOpts *Pagination, vars map[string]any) (string, []interface{}, *paginationContext, error) {
	pCtx, err := s.setupPagination(pToken, pOpts)
	if err != nil {
		return "", nil, nil, err
	}

	q, qArgs, paginationUsed, err := s.parseQueryOpts(pCtx, query, vars)
	if err != nil {
		return "", nil, nil, err
	}

	if !paginationUsed {
		pCtx = nil
	}

	return q, qArgs, pCtx, nil
}

func (s *SQLSyncer) nextPageToken(pCtx *paginationContext, lastRowID any) (string, error) {
	if pCtx == nil {
		return "", nil
	}

	var ret string

	pageSize := int(pCtx.Limit)

	switch pCtx.Strategy {
	case offsetKey:
		ret = strconv.Itoa(int(pCtx.Offset) + pageSize)
	case cursorKey:
		switch l := lastRowID.(type) {
		case string:
			ret = l
		case []byte:
			ret = string(l)
		case int64:
			ret = strconv.FormatInt(l, 10)
		case int:
			ret = strconv.Itoa(l)
		case int32:
			ret = strconv.FormatInt(int64(l), 10)
		case int16:
			ret = strconv.FormatInt(int64(l), 10)
		case int8:
			ret = strconv.FormatInt(int64(l), 10)
		case uint64:
			ret = strconv.FormatUint(l, 10)
		case uint:
			ret = strconv.FormatUint(uint64(l), 10)
		case uint32:
			ret = strconv.FormatUint(uint64(l), 10)
		case uint16:
			ret = strconv.FormatUint(uint64(l), 10)
		case uint8:
			ret = strconv.FormatUint(uint64(l), 10)
		default:
			return "", errors.New("unexpected type for primary key")
		}
	default:
		return "", fmt.Errorf("unexpected pagination strategy: %s", pCtx.Strategy)
	}

	return ret, nil
}

func (s *SQLSyncer) setupPagination(pToken *pagination.Token, pOpts *Pagination) (*paginationContext, error) {
	if pOpts == nil {
		return nil, nil
	}

	ret := &paginationContext{
		Strategy:   pOpts.Strategy,
		PrimaryKey: pOpts.PrimaryKey,
	}

	ret.Limit = clampPageSize(pToken.Size, pOpts.PageSize)

	switch pOpts.Strategy {
	case offsetKey:
		if pToken.Token != "" {
			offset, err := strconv.ParseInt(pToken.Token, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("failed to parse offset token %s: %w", pToken.Token, err)
			}
			ret.Offset = offset
		} else {
			ret.Offset = 0
		}

	case cursorKey:
		ret.Cursor = pToken.Token

	default:
		return nil, fmt.Errorf("unknown pagination strategy %s", pOpts.Strategy)
	}

	return ret, nil
}

func (s *SQLSyncer) prepareProvisioningQuery(query string, vars map[string]any) (string, []interface{}, error) {
	var qArgs []interface{}

	var parseErr error
	updatedQuery := queryOptRegex.ReplaceAllStringFunc(query, func(token string) string {
		opts, err := parseToken(token)
		if err != nil {
			parseErr = errors.Join(parseErr, fmt.Errorf("in token %s: %w", token, err))
			return token
		}

		v, ok := vars[opts.Key]

		if !ok {
			parseErr = errors.Join(parseErr, fmt.Errorf("unknown token %s", token))
			return token
		}

		if opts.Unquoted {
			return SanitizeIdentifier(fmt.Sprintf("%v", v))
		}

		if opts.Identifier {
			return s.quoteIdentifier(fmt.Sprintf("%v", v))
		}

		qArgs = append(qArgs, v)
		return s.getNextPlaceholder(qArgs)
	})
	if parseErr != nil {
		return "", nil, parseErr
	}
	return updatedQuery, qArgs, nil
}

// resolveProvisioningDB routes via vars["database"]. Unset → primary handle (NOT the
// last-iterated handle, which would couple provisioning to sync state). Unknown name
// → loud error, never silent fallthrough to a wrong-cluster GRANT.
func (s *SQLSyncer) resolveProvisioningDB(vars map[string]any) (*sql.DB, error) {
	if raw, ok := vars[rowColDatabase]; ok {
		if name, ok := raw.(string); ok && name != "" {
			db, found := s.dbs[name]
			if !found {
				return nil, fmt.Errorf("provisioning: unknown database %q in vars (configured: %v)", name, s.dbNames)
			}
			return db, nil
		}
	}
	if db, ok := s.dbs[s.primaryDBName]; ok {
		return db, nil
	}
	return nil, fmt.Errorf("provisioning: primary database %q not found in handles (configured: %v)", s.primaryDBName, s.dbNames)
}

func (s *SQLSyncer) RunProvisioningQueries(
	ctx context.Context,
	queries,
	validationQueries []string,
	vars map[string]any,
	useTx bool,
) error {
	l := ctxzap.Extract(ctx).With(
		zap.Bool("use_tx", useTx),
	)

	ctx = ctxzap.ToContext(ctx, l)

	target, err := s.resolveProvisioningDB(vars)
	if err != nil {
		return err
	}

	var committed bool
	var executor executor = target

	if useTx {
		tx, err := target.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		executor = tx

		defer func() {
			if !committed {
				if err := tx.Rollback(); err != nil {
					l.Error("failed to rollback provisioning queries", zap.Error(err))
				}
			}
		}()
	}

	err = s.RunProvisioningQueriesWithExecutor(
		ctx,
		queries,
		validationQueries,
		vars,
		executor,
	)
	if err != nil {
		return err
	}

	if useTx {
		tx, ok := executor.(*sql.Tx)
		if !ok {
			return errors.New("transactional executor required")
		}
		err := tx.Commit()
		if err != nil {
			return err
		}
		committed = true
	}

	return nil
}

// RunRevokeProvisioning runs revoke queries like RunProvisioningQueries, and
// additionally runs an optional principal-exists probe once the revoke has
// committed. The probe detects the case where the revoke deleted the principal
// itself downstream (e.g. removing a user's last role deletes the user row):
// when the exists-check returns no rows, principalDeleted is true.
//
// The probe only decides whether the caller reports the deletion, so it runs
// outside the revoke transaction: a probe failure must not roll back a revoke
// that already succeeded. Probe failures are logged and reported as "not
// deleted", leaving the next sync to pick up the deletion.
//
// When every revoke query affects zero rows the function still commits and
// probes, then returns ErrQueryAffectedZeroRows so the caller can report
// GrantAlreadyRevoked — combined with ResourceDeleted when the principal is
// also gone, so retried revokes still surface the deletion.
func (s *SQLSyncer) RunRevokeProvisioning(
	ctx context.Context,
	queries,
	validationQueries []string,
	existsCheck *PrincipalExistsCheck,
	vars map[string]any,
	useTx bool,
) (bool, error) {
	l := ctxzap.Extract(ctx).With(
		zap.Bool("use_tx", useTx),
	)

	ctx = ctxzap.ToContext(ctx, l)

	target, err := s.resolveProvisioningDB(vars)
	if err != nil {
		return false, err
	}

	allZero, err := s.runRevokeQueries(ctx, queries, validationQueries, vars, useTx, target)
	if err != nil {
		return false, err
	}

	var principalDeleted bool
	if existsCheck != nil {
		exists, err := s.runPrincipalExistsCheck(ctx, target, existsCheck, vars)
		if err != nil {
			l.Warn(
				"revoke succeeded but the principal exists check failed; not reporting a principal deletion",
				zap.Error(err),
			)
			// failed to probe the principal: don't report a deletion, and also don't return an error
			principalDeleted = false
		} else {
			// No rows from the exists-check => the principal was deleted by the revoke.
			principalDeleted = !exists
		}
	}

	if allZero {
		return principalDeleted, ErrQueryAffectedZeroRows
	}
	return principalDeleted, nil
}

// runRevokeQueries executes the revoke queries against target, committing when
// useTx is set. It reports whether every query affected zero rows, which means
// the grant was already revoked; that case commits rather than failing so the
// caller can still probe the principal and annotate the response.
func (s *SQLSyncer) runRevokeQueries(
	ctx context.Context,
	queries,
	validationQueries []string,
	vars map[string]any,
	useTx bool,
	target *sql.DB,
) (bool, error) {
	l := ctxzap.Extract(ctx)

	var committed bool
	var executor executor = target

	if useTx {
		tx, err := target.BeginTx(ctx, nil)
		if err != nil {
			return false, err
		}
		executor = tx

		defer func() {
			if !committed {
				if err := tx.Rollback(); err != nil {
					l.Error("failed to rollback revoke provisioning queries", zap.Error(err))
				}
			}
		}()
	}

	var allZero bool
	err := s.RunProvisioningQueriesWithExecutor(ctx, queries, validationQueries, vars, executor)
	if err != nil {
		if !errors.Is(err, ErrQueryAffectedZeroRows) {
			return false, err
		}
		allZero = true
	}

	if useTx {
		tx, ok := executor.(*sql.Tx)
		if !ok {
			return false, errors.New("transactional executor required")
		}
		if err := tx.Commit(); err != nil {
			return false, err
		}
		committed = true
	}

	return allZero, nil
}

// runPrincipalExistsCheck executes the exists-check probe on the given
// executor. It returns true when the probe returns at least one row (the
// principal still exists); no rows means the principal is gone.
func (s *SQLSyncer) runPrincipalExistsCheck(
	ctx context.Context,
	executor executor,
	existsCheck *PrincipalExistsCheck,
	vars map[string]any,
) (bool, error) {
	l := ctxzap.Extract(ctx)

	q, qArgs, err := s.prepareProvisioningQuery(existsCheck.Query, vars)
	if err != nil {
		return false, fmt.Errorf("failed to prepare principal exists check query: %w", err)
	}

	l.Debug(
		"running principal exists check query",
		zap.String("query", q),
		zap.Any("args", qArgs),
	)

	rows, err := executor.QueryContext(ctx, q, qArgs...)
	if err != nil {
		return false, fmt.Errorf("failed to execute principal exists check query: %w", err)
	}

	exists := rows.Next()

	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return false, fmt.Errorf("failed to read principal exists check result: %w", err)
	}

	if err := rows.Close(); err != nil {
		return false, fmt.Errorf("failed to close principal exists check result: %w", err)
	}

	// >=1 row => the principal still exists.
	return exists, nil
}

func (s *SQLSyncer) RunProvisioningQueriesWithExecutor(
	ctx context.Context,
	queries,
	validationQueries []string,
	vars map[string]any,
	executor executor,
) error {
	l := ctxzap.Extract(ctx)

	for _, q := range validationQueries {
		q, qArgs, err := s.prepareProvisioningQuery(q, vars)
		if err != nil {
			return fmt.Errorf("failed to prepare validation query: %w", err)
		}

		l.Debug(
			"running validation query",
			zap.String("query", q),
			zap.Any("args", qArgs),
			zap.Any("vars", vars),
		)

		result, err := executor.QueryContext(ctx, q, qArgs...)
		if err != nil {
			return fmt.Errorf("failed to execute validation query: %w", err)
		}

		valid := result.Next()

		if err := result.Err(); err != nil {
			return fmt.Errorf("failed to read validation query result: %w", err)
		}

		err = result.Close()
		if err != nil {
			return fmt.Errorf("failed to close validation query result: %w", err)
		}

		if !valid {
			return fmt.Errorf("validation query returned no rows")
		}
	}

	zeroRowCount := 0

	for idx, q := range queries {
		q, qArgs, err := s.prepareProvisioningQuery(q, vars)
		if err != nil {
			return fmt.Errorf("failed to prepare query: %w", err)
		}

		result, err := executor.ExecContext(ctx, q, qArgs...)
		if err != nil {
			return fmt.Errorf("failed to execute query: %w", err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			l.Error("failed to get rows affected", zap.Error(err))
		}

		if rowsAffected > 1 {
			return fmt.Errorf("provisioning query at index %d affected %d rows: %w", idx, rowsAffected, ErrQueryAffectedMoreThanOneRow)
		}

		if rowsAffected == 0 {
			zeroRowCount++
		}

		l.Debug("query executed", zap.String("query", q), zap.Any("args", qArgs), zap.Int64("rows_affected", rowsAffected))
	}

	if len(queries) > 0 && zeroRowCount == len(queries) {
		return ErrQueryAffectedZeroRows
	}

	return nil
}

func (s *SQLSyncer) PrepareQueryVars(ctx context.Context, inputs map[string]any, vars map[string]string) (map[string]any, error) {
	ret := make(map[string]any)

	if inputs == nil {
		inputs = make(map[string]any)
	}

	for k, v := range vars {
		// Check if the value is a direct reference to an input field
		if inputVal, exists := inputs[v]; exists {
			normalizedValue := s.normalizeValue(inputVal)
			ret[k] = normalizedValue
			continue
		}

		// Otherwise, evaluate it as a CEL expression
		out, err := s.env.Evaluate(ctx, v, inputs)
		if err != nil {
			return nil, err
		}
		normalizedValue := s.normalizeValue(out)
		ret[k] = normalizedValue
	}

	return ret, nil
}

// normalizeValue converts CEL null types and other special values to Go nil for SQL compatibility
// Also converts booleans to strings ("1"/"0") for Oracle compatibility when used in DECODE statements.
func (s *SQLSyncer) normalizeValue(val any) any {
	if val == nil {
		return nil
	}

	// Check for CEL null types
	switch v := val.(type) {
	case string:
		return v
	case types.Null:
		// CEL Null type
		return nil
	case bool:
		// Convert boolean to string for Oracle compatibility (Oracle DECODE expects CHAR)
		// Only convert for Oracle to avoid breaking other databases
		if s.dbEngine == database.Oracle {
			result := "0"
			if v {
				result = "1"
			}
			return result
		}
		if s.dbEngine == database.MSSQL {
			result := 0
			if v {
				result = 1
			}
			return result
		}
		// For other databases, return as-is (let the driver handle it)
		return v
	}

	// Use reflection to check for CEL null value types
	valType := reflect.TypeOf(val)
	if valType != nil {
		typeName := valType.String()
		// Check for CEL null value types
		if strings.Contains(typeName, "NullValue") || strings.Contains(typeName, "types.Null") {
			return nil
		}
	}

	return val
}

func (s *SQLSyncer) runQuery(
	ctx context.Context,
	db executor,
	pToken *pagination.Token,
	query string,
	pOpts *Pagination,
	vars map[string]any,
	rowCallback func(context.Context, map[string]interface{}) (bool, error),
) (string, error) {
	l := ctxzap.Extract(ctx)

	q, qArgs, pCtx, err := s.prepareQuery(pToken, query, pOpts, vars)
	if err != nil {
		return "", err
	}

	l.Debug("running query", zap.String("query", q), zap.Any("args", qArgs))

	rows, err := db.QueryContext(ctx, q, qArgs...)
	if err != nil {
		l.Error("failed to run query", zap.String("query", q), zap.Any("args", qArgs), zap.Error(err))
		return "", err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return "", err
	}

	values := make([]interface{}, len(columns))
	scanArgs := make([]interface{}, len(values))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	var lastRowID any
	rowCount := 0
	for rows.Next() {
		rowCount++

		if pCtx != nil && rowCount > int(pCtx.Limit) {
			break
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return "", err
		}

		foundPaginationKey := false
		rowMap := make(map[string]interface{})
		for i, colName := range columns {
			rowMap[colName] = values[i]
			if pCtx != nil && pCtx.PrimaryKey == colName {
				lastRowID = values[i]
				foundPaginationKey = true
			}
		}

		// Real result columns named "database" win over the synthetic injection.
		if _, exists := rowMap[rowColDatabase]; !exists && s.currentDBName != "" {
			rowMap[rowColDatabase] = s.currentDBName
		}

		if pCtx != nil && !foundPaginationKey {
			return "", errors.New("primary key not found in query results")
		}

		ok, err := rowCallback(ctx, rowMap)
		if err != nil {
			return "", err
		}
		if !ok {
			break
		}
	}

	if err := rows.Err(); err != nil {
		return "", err
	}

	nextPageToken := ""
	if pCtx != nil && rowCount > int(pCtx.Limit) {
		nextPageToken, err = s.nextPageToken(pCtx, lastRowID)
		if err != nil {
			return "", err
		}
	}

	return nextPageToken, nil
}

func (s *SQLSyncer) runGrantRejectIf(
	ctx context.Context,
	executor executor,
	rejectIf *GrantRejectIfProvisioningQuery,
	vars map[string]any,
) (bool, string, error) {
	if rejectIf == nil || rejectIf.Query == "" {
		return false, "", nil
	}

	var cancelled bool
	reason := defaultGrantCancelledReason

	_, err := s.runQuery(ctx, executor, nil, rejectIf.Query, nil, vars, func(ctx context.Context, rowMap map[string]interface{}) (bool, error) {
		cancelled = true

		if rejectIf.Reason != "" {
			out, err := s.env.EvaluateString(ctx, rejectIf.Reason, s.env.SyncInputs(rowMap))
			if err != nil {
				return false, fmt.Errorf("failed to evaluate grant cancellation reason: %w", err)
			}
			reason = out
		}

		return false, nil
	})
	if err != nil {
		return false, "", err
	}

	return cancelled, reason, nil
}

func (s *SQLSyncer) RunGrantProvisioning(
	ctx context.Context,
	resource *v2.Resource,
	queries,
	validationQueries []string,
	vars map[string]any,
	useTx bool,
	replace *GrantReplaceProvisioningQueries,
	rejectIf *GrantRejectIfProvisioningQuery,
) (annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)

	anno := annotations.New()

	target, err := s.resolveProvisioningDB(vars)
	if err != nil {
		return nil, err
	}

	var committed bool
	var executor executor = target

	if useTx {
		tx, err := target.BeginTx(ctx, nil)
		if err != nil {
			return anno, err
		}
		executor = tx

		defer func() {
			if !committed {
				if err := tx.Rollback(); err != nil {
					l.Error("failed to rollback provisioning queries", zap.Error(err))
				}
			}
		}()
	}

	cancelled, reason, err := s.runGrantRejectIf(ctx, executor, rejectIf, vars)
	if err != nil {
		return anno, err
	}
	if cancelled {
		l.Info("grant cancelled by connector policy", zap.String("reason", reason))
		return anno, sdkGrant.NewErrGrantCancelled(reason)
	}

	if replace != nil && replace.Query != "" {
		l.Info("running grant replace provisioning query", zap.String("query", replace.Query))

		var ret []*v2.Grant

		_, err := s.runQuery(ctx, executor, nil, replace.Query, nil, vars, func(ctx context.Context, rowMap map[string]any) (bool, error) {
			for _, mapping := range replace.Map {
				if mapping.EntitlementResourceId == "" {
					continue
				}

				if mapping.SkipIf != "" {
					skip, err := s.env.EvaluateBool(ctx, mapping.SkipIf, rowMap)
					if err != nil {
						return false, fmt.Errorf("failed to evaluate skip_if: %w", err)
					}

					if skip {
						continue
					}
				}

				entitlementResourceId, err := s.env.EvaluateString(ctx, mapping.EntitlementResourceId, s.env.SyncInputs(rowMap))
				if err != nil {
					l.Debug(
						"failed to evaluate entitlement resource ID for grant replace provisioning query",
						zap.String("expression", mapping.EntitlementResourceId),
						zap.Any("row", s.env.SyncInputs(rowMap)),
						zap.Error(err),
					)
					return false, err
				}

				entitlementResource := &v2.Resource{
					Id: &v2.ResourceId{
						ResourceType: s.resourceType.GetId(),
						Resource:     entitlementResourceId,
					},
				}

				// Resource Should be entitlement.Resource
				g, ok, err := s.mapGrant(ctx, entitlementResource, mapping, rowMap)
				if err != nil {
					return false, err
				}

				if ok {
					ret = append(ret, g)
				}
			}
			return true, nil
		})
		if err != nil {
			return anno, err
		}

		switch {
		case len(ret) > 1:
			return nil, fmt.Errorf("grant provisioning query returned %d rows, expected at most 1: %w", len(ret), ErrQueryAffectedMoreThanOneRow)
		case len(ret) == 1:
			// Revoke grant
			grantToRevoke := ret[0]

			l.Info(
				"grant provisioning query returned a grant to replace",
				zap.String("grant_id", grantToRevoke.GetId()),
				zap.String("principal_id", grantToRevoke.GetPrincipal().GetId().GetResource()),
				zap.String("entitlement_id", grantToRevoke.GetEntitlement().GetId()),
			)

			_, _, entitlementID, err := helpers.SplitEntitlementID(grantToRevoke.GetEntitlement())
			if err != nil {
				return anno, err
			}

			provisioningConfig, ok := s.getProvisioningConfig(ctx, entitlementID)
			if !ok {
				return anno, errors.New("provisioning is not enabled for this connector")
			}

			if provisioningConfig.Revoke == nil {
				return anno, errors.New("no revoke config found for entitlement")
			}

			if len(provisioningConfig.Revoke.Queries) == 0 {
				return anno, errors.New("no revoke config found for entitlement")
			}

			provisioningVars, err := s.prepareProvisioningVars(
				ctx,
				provisioningConfig.Vars,
				grantToRevoke.GetPrincipal(),
				grantToRevoke.GetEntitlement(),
			)
			if err != nil {
				return anno, err
			}

			err = s.RunProvisioningQueriesWithExecutor(
				ctx,
				provisioningConfig.Revoke.Queries,
				provisioningConfig.Revoke.ValidationQueries,
				provisioningVars,
				executor,
			)
			if err != nil {
				if !errors.Is(err, ErrQueryAffectedZeroRows) {
					return anno, err
				}
			}

			anno.Update(&v2.GrantReplaced{
				ReplacedGrantId: grantToRevoke.GetId(),
			})

		case len(ret) == 0:
		}
	}

	for _, q := range validationQueries {
		q, qArgs, err := s.prepareProvisioningQuery(q, vars)
		if err != nil {
			return anno, fmt.Errorf("failed to prepare validation query: %w", err)
		}

		result, err := executor.QueryContext(ctx, q, qArgs...)
		if err != nil {
			return anno, fmt.Errorf("failed to execute validation query: %w", err)
		}

		valid := result.Next()

		if err := result.Err(); err != nil {
			_ = result.Close()
			return anno, fmt.Errorf("failed to read validation query result: %w", err)
		}

		err = result.Close()
		if err != nil {
			return anno, fmt.Errorf("failed to close validation query result: %w", err)
		}

		if !valid {
			return anno, fmt.Errorf("grant provisioning: validation query returned no rows")
		}
	}

	zeroRowCount := 0

	for idx, q := range queries {
		q, qArgs, err := s.prepareProvisioningQuery(q, vars)
		if err != nil {
			return anno, fmt.Errorf("failed to prepare query: %w", err)
		}

		result, err := executor.ExecContext(ctx, q, qArgs...)
		if err != nil {
			return anno, fmt.Errorf("failed to execute query: %w", err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			l.Error("failed to get rows affected", zap.Error(err))
		}

		if rowsAffected > 1 {
			return anno, fmt.Errorf("provisioning query at index %d affected %d rows: %w", idx, rowsAffected, ErrQueryAffectedMoreThanOneRow)
		}

		if rowsAffected == 0 {
			zeroRowCount++
		}

		l.Debug("query executed", zap.String("query", q), zap.Any("args", qArgs), zap.Int64("rows_affected", rowsAffected), zap.Bool("use_tx", useTx))
	}

	if len(queries) > 0 && zeroRowCount == len(queries) {
		return anno, ErrQueryAffectedZeroRows
	}

	if useTx {
		tx, ok := executor.(*sql.Tx)
		if !ok {
			return anno, errors.New("transactional executor required")
		}
		err := tx.Commit()
		if err != nil {
			return anno, err
		}
		committed = true
	}

	return anno, nil
}
