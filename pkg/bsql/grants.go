package bsql

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	sdkGrant "github.com/conductorone/baton-sdk/pkg/types/grant"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
)

func (s *SQLSyncer) Grants(ctx context.Context, resource *v2.Resource, pToken *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	if len(s.config.Grants) == 0 {
		return nil, "", nil, nil
	}

	var ret []*v2.Grant

	// TODO(jirwin): Better pagination support for multiple grants
	//for _, g := range s.config.Grants {
	grants, npt, err := s.listGrants(ctx, resource, pToken, s.config.Grants[0])
	if err != nil {
		return nil, "", nil, err
	}

	ret = append(ret, grants...)
	//}

	return ret, npt, nil, nil
}

func (s *SQLSyncer) listGrants(ctx context.Context, resource *v2.Resource, pToken *pagination.Token, grantConfig *GrantsQuery) ([]*v2.Grant, string, error) {
	var ret []*v2.Grant

	if grantConfig == nil {
		return nil, "", errors.New("error: missing grants query")
	}

	q, qArgs, pCtx, err := s.prepareQuery(ctx, pToken, grantConfig.Query, grantConfig.Pagination)
	if err != nil {
		return nil, "", err
	}

	rows, err := s.db.QueryContext(ctx, q, qArgs...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, "", err
	}

	values := make([]interface{}, len(columns))
	scanArgs := make([]interface{}, len(values))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	pageSize := int(pCtx.Limit)
	var lastRowID interface{}
	rowCount := 0
	for rows.Next() {
		rowCount++

		if rowCount > pageSize {
			break
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return nil, "", err
		}

		foundPaginationKey := false
		rowMap := make(map[string]interface{})
		for i, colName := range columns {
			rowMap[colName] = values[i]
			if grantConfig.Pagination.PrimaryKey == colName {
				lastRowID = values[i]
				foundPaginationKey = true
			}
		}

		if !foundPaginationKey {
			return nil, "", errors.New("primary key not found in query result")
		}

		g, ok, err := s.mapGrant(ctx, resource, grantConfig.Map, rowMap)
		if err != nil {
			return nil, "", err
		}
		if ok {
			ret = append(ret, g)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	nextPageToken := ""
	if rowCount > pageSize {
		switch grantConfig.Pagination.Strategy {
		case "offset":
			nextPageToken = strconv.Itoa(int(pCtx.Offset)*pageSize + pageSize)
		case "cursor":
			switch l := lastRowID.(type) {
			case string:
				nextPageToken = l
			case []byte:
				nextPageToken = string(l)
			case int64:
				nextPageToken = strconv.FormatInt(l, 10)
			case int:
				nextPageToken = strconv.Itoa(l)
			case int32:
				nextPageToken = strconv.FormatInt(int64(l), 10)
			case int16:
				nextPageToken = strconv.FormatInt(int64(l), 10)
			case int8:
				nextPageToken = strconv.FormatInt(int64(l), 10)
			case uint64:
				nextPageToken = strconv.FormatUint(l, 10)
			case uint:
				nextPageToken = strconv.FormatUint(uint64(l), 10)
			case uint32:
				nextPageToken = strconv.FormatUint(uint64(l), 10)
			case uint16:
				nextPageToken = strconv.FormatUint(uint64(l), 10)
			case uint8:
				nextPageToken = strconv.FormatUint(uint64(l), 10)
			default:
				return nil, "", errors.New("unexpected type for primary key")
			}
		default:
			return nil, "", fmt.Errorf("unexpected pagination strategy: %s", grantConfig.Pagination.Strategy)
		}
	}

	return ret, nextPageToken, nil
}

func (s *SQLSyncer) mapGrant(ctx context.Context, resource *v2.Resource, mapping *GrantMapping, rowMap map[string]any) (*v2.Grant, bool, error) {
	if mapping == nil {
		return nil, false, errors.New("error: missing grant mapping")
	}

	if mapping.PrincipalId == "" {
		return nil, false, errors.New("error: missing principal ID mapping")
	}

	if mapping.PrincipalType == "" {
		return nil, false, errors.New("error: missing principal type mapping")
	}

	if mapping.Entitlement == "" {
		return nil, false, errors.New("error: missing entitlement ID mapping")
	}

	inputs := s.env.BaseInputsWithResource(rowMap, resource)

	if mapping.SkipIf != "" {
		skip, err := s.env.EvaluateBool(ctx, mapping.SkipIf, inputs)
		if err != nil {
			return nil, false, err
		}

		if skip {
			return nil, false, nil
		}
	}

	principalID, err := s.env.EvaluateString(ctx, mapping.PrincipalId, inputs)
	if err != nil {
		return nil, false, err
	}

	principalType := mapping.PrincipalType

	principal := &v2.ResourceId{
		ResourceType: principalType,
		Resource:     principalID,
	}

	return sdkGrant.NewGrant(resource, mapping.Entitlement, principal), true, nil
}
