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

	b := &pagination.Bag{}
	err := b.Unmarshal(pToken.Token)
	if err != nil {
		return nil, "", nil, err
	}

	if b.Current() == nil {
		for ii := range s.config.Grants {
			b.Push(pagination.PageState{
				ResourceTypeID: "grant-query",
				ResourceID:     strconv.Itoa(ii),
			})
		}
	}

	current := b.Current()
	switch current.ResourceTypeID {
	case "grant-query":
		grantIi, err := strconv.ParseInt(current.ResourceID, 10, 64)
		if err != nil {
			return nil, "", nil, err
		}

		grants, npt, err := s.listGrants(ctx, resource, &pagination.Token{
			Size:  pToken.Size,
			Token: current.Token,
		}, s.config.Grants[grantIi])
		if err != nil {
			return nil, "", nil, err
		}
		err = b.Next(npt)
		if err != nil {
			return nil, "", nil, err
		}

		ret = append(ret, grants...)

	default:
		return nil, "", nil, errors.New("invalid page token")
	}

	nextPageToken, err := b.Marshal()
	if err != nil {
		return nil, "", nil, err
	}

	return ret, nextPageToken, nil, nil
}

func (s *SQLSyncer) listGrants(ctx context.Context, resource *v2.Resource, pToken *pagination.Token, grantConfig *GrantsQuery) ([]*v2.Grant, string, error) {
	if grantConfig == nil {
		return nil, "", errors.New("error: missing grants query")
	}

	var ret []*v2.Grant

	inputs := s.env.SyncInputsWithResource(nil, resource)

	queryVars, err := s.prepareQueryVars(ctx, inputs, grantConfig.Vars)
	if err != nil {
		return nil, "", err
	}

	npt, err := s.runQuery(ctx, pToken, grantConfig.Query, grantConfig.Pagination, queryVars, func(ctx context.Context, rowMap map[string]any) (bool, error) {
		for _, mapping := range grantConfig.Map {
			g, ok, err := s.mapGrant(ctx, resource, mapping, rowMap)
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

	inputs := s.env.SyncInputsWithResource(rowMap, resource)

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

	entitlementID, err := s.env.EvaluateString(ctx, mapping.Entitlement, inputs)
	if err != nil {
		return nil, false, err
	}

	grantOptions := []sdkGrant.GrantOption{}
	if mapping.Expandable != nil {
		skip := false
		if mapping.Expandable.SkipIf != "" {
			skip, err = s.env.EvaluateBool(ctx, mapping.Expandable.SkipIf, inputs)
			if err != nil {
				return nil, false, err
			}
		}

		if !skip {
			entitlementIDs := []string{}
			for _, entitlement := range mapping.Expandable.Entitlements {
				entitlementID, err := s.env.EvaluateString(ctx, entitlement, inputs)
				if err != nil {
					return nil, false, err
				}
				entitlementIDs = append(entitlementIDs, entitlementID)
			}

			grantOptions = append(grantOptions, sdkGrant.WithAnnotation(&v2.GrantExpandable{
				EntitlementIds: entitlementIDs,
				Shallow:        mapping.Expandable.Shallow,
			}))
		}
	}

	return sdkGrant.NewGrant(resource, entitlementID, principal, grantOptions...), true, nil
}
