package bsql

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"

	"github.com/conductorone/baton-sdk/pkg/pagination"
)

const (
	maxPageSize = 1000
	minPageSize = 1
)

type paginationContext struct {
	Strategy string
	Limit    int64
	Offset   int64
	Cursor   string
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

func (s *SQLSyncer) parseQueryOpts(ctx context.Context, pCtx *paginationContext) (string, []interface{}, error) {
	if pCtx == nil {
		return s.config.List.Query, nil, nil
	}

	var qArgs []interface{}

	var parseErr error
	updatedQuery := queryOptRegex.ReplaceAllStringFunc(s.config.List.Query, func(token string) string {
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

func (s *SQLSyncer) prepareQuery(ctx context.Context, pToken *pagination.Token) (string, []interface{}, *paginationContext, error) {
	if s.config.List == nil {
		return "", nil, nil, errors.New("missing list configuration")
	}

	pCtx, err := s.setupPagination(ctx, pToken)
	if err != nil {
		return "", nil, nil, err
	}

	q, qArgs, err := s.parseQueryOpts(ctx, pCtx)
	if err != nil {
		return "", nil, nil, err
	}

	spew.Dump(q, qArgs, pCtx, pToken)

	return q, qArgs, pCtx, nil
}

func (s *SQLSyncer) setupPagination(ctx context.Context, pToken *pagination.Token) (*paginationContext, error) {
	if s.config.List.Pagination == nil {
		return nil, nil
	}
	pConfig := s.config.List.Pagination

	ret := &paginationContext{
		Strategy: pConfig.Strategy,
	}

	ret.Limit = clampPageSize(pToken.Size)

	switch pConfig.Strategy {
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
		return nil, fmt.Errorf("unknown pagination strategy %s", pConfig.Strategy)
	}

	return ret, nil
}
