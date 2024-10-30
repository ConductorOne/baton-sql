package bsql

import (
	"context"
	"errors"
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

	for _, g := range s.config.Grants {
		grants, err := s.listGrants(ctx, resource, pToken, g)
		if err != nil {
			return nil, "", nil, err
		}

		ret = append(ret, grants...)
	}

	return ret, "", nil, nil
}

func (s *SQLSyncer) listGrants(ctx context.Context, resource *v2.Resource, pToken *pagination.Token, grantConfig *GrantsQuery) ([]*v2.Grant, error) {
	var ret []*v2.Grant

	limit := pToken.Size
	if limit == 0 {
		limit = 100
	}

	qCtx := map[string]string{
		"limit": strconv.Itoa(limit),
	}

	if pToken.Token != "" {
		qCtx["offset"] = pToken.Token
	} else {
		qCtx["offset"] = "0"
	}

	q, err := parseQueryOpts(ctx, grantConfig.Query, qCtx)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	values := make([]interface{}, len(columns))
	scanArgs := make([]interface{}, len(values))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	for rows.Next() {
		if err := rows.Scan(scanArgs...); err != nil {
			return nil, err
		}

		rowMap := make(map[string]interface{})
		for i, colName := range columns {
			rowMap[colName] = values[i]
		}

		g, ok, err := s.mapGrant(ctx, resource, grantConfig.Map, rowMap)
		if err != nil {
			return nil, err
		}
		if ok {
			ret = append(ret, g)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ret, nil
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
