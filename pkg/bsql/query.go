package bsql

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/conductorone/baton-sdk/pkg/pagination"
)

const (
	maxPageSize = 1000
	minPageSize = 1
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
	switch s.dbType {
	case "mysql":
		return "?"
	case "postgres":
		return fmt.Sprintf("$%d", len(qArgs)+1)
	case "oracle":
		return fmt.Sprintf(":%d", len(qArgs)+1)
	default:
		return "?"
	}
}

func (s *SQLSyncer) parseQueryOpts(ctx context.Context, pCtx *paginationContext, query string) (string, []interface{}, error) {
	if pCtx == nil {
		return query, nil, nil
	}

	var qArgs []interface{}

	var parseErr error
	updatedQuery := queryOptRegex.ReplaceAllStringFunc(query, func(token string) string {
		key := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(token, "?<"), ">"))

		switch key {
		case "limit":
			// Always request 1 more than the specified limit, so we can see if there are additional results.
			qArgs = append(qArgs, pCtx.Limit+1)
		case "offset":
			qArgs = append(qArgs, pCtx.Offset)
		case "cursor":
			qArgs = append(qArgs, pCtx.Cursor)
		default:
			parseErr = errors.Join(parseErr, fmt.Errorf("unknown token %s", token))
			return token
		}

		return s.getNextPlaceholder(ctx, qArgs)
	})
	if parseErr != nil {
		return "", nil, parseErr
	}
	return updatedQuery, qArgs, nil
}

func clampPageSize(pageSize int) int64 {
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

	q, qArgs, err := s.parseQueryOpts(ctx, pCtx, query)
	if err != nil {
		return "", nil, nil, err
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
	case "offset":
		ret = strconv.Itoa(int(pCtx.Offset)*pageSize + pageSize)
	case "cursor":
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
	case "offset":
		if pToken.Token != "" {
			offset, err := strconv.ParseInt(pToken.Token, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("failed to parse offset token %s: %w", pToken.Token, err)
			}
			ret.Offset = offset
		} else {
			ret.Offset = 0
		}

	case "cursor":
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
	q, qArgs, pCtx, err := s.prepareQuery(ctx, pToken, query, pOpts)
	if err != nil {
		return "", err
	}

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

	pageSize := int(pCtx.Limit)
	var lastRowID any
	rowCount := 0
	for rows.Next() {
		rowCount++

		if rowCount > pageSize {
			break
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return "", err
		}

		foundPaginationKey := false
		rowMap := make(map[string]interface{})
		for i, colName := range columns {
			rowMap[colName] = values[i]
			if pCtx.PrimaryKey == colName {
				lastRowID = values[i]
				foundPaginationKey = true
			}
		}

		if !foundPaginationKey {
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
	if rowCount > pageSize {
		nextPageToken, err = s.nextPageToken(ctx, pCtx, lastRowID)
		if err != nil {
			return "", err
		}
	}

	return nextPageToken, nil
}
