package bsql

import (
	"context"
	"errors"

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
	if grantConfig == nil {
		return nil, "", errors.New("error: missing grants query")
	}

	var ret []*v2.Grant

	npt, err := s.runQuery(ctx, pToken, grantConfig.Query, grantConfig.Pagination, func(ctx context.Context, rowMap map[string]any) (bool, error) {
		g, ok, err := s.mapGrant(ctx, resource, grantConfig.Map, rowMap)
		if err != nil {
			return false, err
		}
		if ok {
			ret = append(ret, g)
		}
		return true, nil
	})
	if err != nil {
		return nil, "", err
	}

	return ret, npt, nil
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
