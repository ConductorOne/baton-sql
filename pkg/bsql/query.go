package bsql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

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
)

type executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type paginationContext struct {
	Strategy   string
	Limit      int64
	Offset     int64
	Cursor     string
	PrimaryKey string
}

type queryTokenOpts struct {
	Key      string
	Unquoted bool
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
		default:
			return nil, fmt.Errorf("unknown option %s", opt)
		}
	}

	return opts, nil
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
			return fmt.Sprintf("%v", val)
		}

		qArgs = append(qArgs, val)
		return s.getNextPlaceholder(qArgs)
	})
	if parseErr != nil {
		return "", nil, false, parseErr
	}
	return updatedQuery, qArgs, paginationOptSet, nil
}

func clampPageSize(pageSize int) int64 {
	if pageSize == 0 {
		return defaultPageSize
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
		ret = strconv.Itoa(int(pCtx.Offset)*pageSize + pageSize)
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

	ret.Limit = clampPageSize(pToken.Size)

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
			return fmt.Sprintf("%v", v)
		}

		qArgs = append(qArgs, v)
		return s.getNextPlaceholder(qArgs)
	})
	if parseErr != nil {
		return "", nil, parseErr
	}
	return updatedQuery, qArgs, nil
}

func (s *SQLSyncer) runProvisioningQueries(ctx context.Context, queries []string, vars map[string]any, useTx bool) error {
	l := ctxzap.Extract(ctx)

	var committed bool
	var executor executor = s.db

	if useTx {
		tx, err := s.db.BeginTx(ctx, nil)
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

	for _, q := range queries {
		q, qArgs, err := s.prepareProvisioningQuery(q, vars)
		if err != nil {
			return err
		}

		result, err := executor.ExecContext(ctx, q, qArgs...)
		if err != nil {
			return err
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			l.Error("failed to get rows affected", zap.Error(err))
		}

		if rowsAffected > 1 {
			return errors.New("query affected more than one row, ending and rolling back")
		}

		l.Debug("query executed", zap.String("query", q), zap.Any("args", qArgs), zap.Int64("rows_affected", rowsAffected), zap.Bool("use_tx", useTx))
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

func (s *SQLSyncer) prepareQueryVars(ctx context.Context, inputs map[string]any, vars map[string]string) (map[string]any, error) {
	ret := make(map[string]any)

	if inputs == nil {
		inputs = make(map[string]any)
	}

	for k, v := range vars {
		// Check if the value is a direct reference to an input field
		if inputVal, exists := inputs[v]; exists {
			ret[k] = inputVal
			continue
		}

		// Otherwise, evaluate it as a CEL expression
		out, err := s.env.Evaluate(ctx, v, inputs)
		if err != nil {
			return nil, err
		}
		ret[k] = out
	}

	return ret, nil
}

func (s *SQLSyncer) runQuery(
	ctx context.Context,
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

	rows, err := s.db.QueryContext(ctx, q, qArgs...)
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
