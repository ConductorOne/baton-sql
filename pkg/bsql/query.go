package bsql

import (
	"context"
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
)

type paginationContext struct {
	Strategy   string
	Limit      int64
	Offset     int64
	Cursor     string
	PrimaryKey string
}

var queryOptRegex = regexp.MustCompile(`\?\<([a-zA-Z0-9_]+)\>`)

func (s *SQLSyncer) getNextPlaceholder(ctx context.Context, qArgs []interface{}) string {
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

func (s *SQLSyncer) parseQueryOpts(ctx context.Context, pCtx *paginationContext, query string) (string, []interface{}, bool, error) {
	if pCtx == nil {
		return query, nil, false, nil
	}

	var qArgs []interface{}

	var parseErr error
	paginationOptSet := false
	updatedQuery := queryOptRegex.ReplaceAllStringFunc(query, func(token string) string {
		key := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(token, "?<"), ">"))

		switch key {
		case limitKey:
			// Always request 1 more than the specified limit, so we can see if there are additional results.
			qArgs = append(qArgs, pCtx.Limit+1)
			paginationOptSet = true
		case offsetKey:
			qArgs = append(qArgs, pCtx.Offset)
			paginationOptSet = true
		case cursorKey:
			qArgs = append(qArgs, pCtx.Cursor)
			paginationOptSet = true
		default:
			parseErr = errors.Join(parseErr, fmt.Errorf("unknown token %s", token))
			return token
		}

		return s.getNextPlaceholder(ctx, qArgs)
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

func (s *SQLSyncer) prepareQuery(ctx context.Context, pToken *pagination.Token, query string, pOpts *Pagination) (string, []interface{}, *paginationContext, error) {
	pCtx, err := s.setupPagination(ctx, pToken, pOpts)
	if err != nil {
		return "", nil, nil, err
	}

	q, qArgs, paginationUsed, err := s.parseQueryOpts(ctx, pCtx, query)
	if err != nil {
		return "", nil, nil, err
	}

	if !paginationUsed {
		pCtx = nil
	}

	return q, qArgs, pCtx, nil
}

func (s *SQLSyncer) nextPageToken(ctx context.Context, pCtx *paginationContext, lastRowID any) (string, error) {
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

func (s *SQLSyncer) setupPagination(ctx context.Context, pToken *pagination.Token, pOpts *Pagination) (*paginationContext, error) {
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

func (s *SQLSyncer) runQuery(
	ctx context.Context,
	pToken *pagination.Token,
	query string,
	pOpts *Pagination,
	rowCallback func(context.Context, map[string]interface{}) (bool, error),
) (string, error) {
	l := ctxzap.Extract(ctx)

	q, qArgs, pCtx, err := s.prepareQuery(ctx, pToken, query, pOpts)
	if err != nil {
		return "", err
	}

	l.Debug("running query", zap.String("query", q), zap.Any("args", qArgs))

	rows, err := s.db.QueryContext(ctx, q, qArgs...)
	if err != nil {
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
		nextPageToken, err = s.nextPageToken(ctx, pCtx, lastRowID)
		if err != nil {
			return "", err
		}
	}

	return nextPageToken, nil
}
