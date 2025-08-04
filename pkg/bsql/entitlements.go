package bsql

import (
	"context"
	"fmt"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	sdkEntitlement "github.com/conductorone/baton-sdk/pkg/types/entitlement"
)

func (s *SQLSyncer) Entitlements(ctx context.Context, resource *v2.Resource, pToken *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	// If we have static entitlements defined, only return those, else return dynamic entitlements
	if s.config.StaticEntitlements != nil {
		return s.staticEntitlements(ctx, resource, pToken)
	}

	return s.dynamicEntitlements(ctx, resource, pToken)
}

func (s *SQLSyncer) staticEntitlements(ctx context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	inputs := s.env.SyncInputsWithResource(nil, resource)

	var ret []*v2.Entitlement
	for _, e := range s.config.StaticEntitlements {
		entitlement := &v2.Entitlement{
			Id:       sdkEntitlement.NewEntitlementID(resource, e.Id),
			Resource: resource,
		}

		// If the slug isn't set, default it to be the same as the ID
		if e.Slug == "" {
			entitlement.Slug = e.Id
		}

		if e.DisplayName == "" {
			return nil, "", nil, fmt.Errorf("static entitlements mapping display_name is required")
		}

		v, err := s.env.EvaluateString(ctx, e.DisplayName, inputs)
		if err != nil {
			return nil, "", nil, err
		}
		entitlement.DisplayName = v

		if e.Description != "" {
			v, err := s.env.EvaluateString(ctx, e.Description, inputs)
			if err != nil {
				return nil, "", nil, err
			}
			entitlement.Description = v
		}

		switch e.Purpose {
		case "assignment":
			entitlement.Purpose = v2.Entitlement_PURPOSE_VALUE_ASSIGNMENT
		case "permission":
			entitlement.Purpose = v2.Entitlement_PURPOSE_VALUE_PERMISSION
		default:
			entitlement.Purpose = v2.Entitlement_PURPOSE_VALUE_UNSPECIFIED
		}

		annos := annotations.Annotations(entitlement.Annotations)
		if e.Immutable {
			annos.Update(&v2.EntitlementImmutable{})
		}
		entitlement.Annotations = annos
		ret = append(ret, entitlement)
	}

	return ret, "", nil, nil
}

func (s *SQLSyncer) dynamicEntitlements(ctx context.Context, resource *v2.Resource, pToken *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	if s.config.Entitlements == nil {
		return nil, "", nil, nil
	}

	var ret []*v2.Entitlement

	inputs := s.env.SyncInputsWithResource(nil, resource)

	queryVars, err := s.prepareQueryVars(ctx, inputs, s.config.Entitlements.Vars)
	if err != nil {
		return nil, "", nil, err
	}

	npt, err := s.runQuery(ctx, pToken, s.config.Entitlements.Query, s.config.Entitlements.Pagination, queryVars, func(ctx context.Context, rowMap map[string]any) (bool, error) {
		for _, mapping := range s.config.Entitlements.Map {
			r, ok, err := s.mapEntitlement(ctx, resource, mapping, rowMap)
			if err != nil {
				return false, err
			}

			if ok {
				r.Resource = resource
				ret = append(ret, r)
			}
		}
		return true, nil
	})
	if err != nil {
		return nil, "", nil, err
	}

	return ret, npt, nil, nil
}

func (s *SQLSyncer) mapEntitlement(ctx context.Context, resource *v2.Resource, mappings *EntitlementMapping, rowMap map[string]any) (*v2.Entitlement, bool, error) {
	ret := &v2.Entitlement{}

	inputs := s.env.SyncInputsWithResource(rowMap, resource)

	if mappings.SkipIf != "" {
		skip, err := s.env.EvaluateBool(ctx, mappings.SkipIf, inputs)
		if err != nil {
			return nil, false, err
		}

		if skip {
			return nil, false, nil
		}
	}

	if mappings.Id == "" {
		return nil, false, fmt.Errorf("entitlements mapping id is required")
	}
	v, err := s.env.EvaluateString(ctx, mappings.Id, inputs)
	if err != nil {
		return nil, false, err
	}
	ret.Id = sdkEntitlement.NewEntitlementID(resource, v)

	if mappings.DisplayName == "" {
		return nil, false, fmt.Errorf("entitlements mapping display_name is required")
	}
	v, err = s.env.EvaluateString(ctx, mappings.DisplayName, inputs)
	if err != nil {
		return nil, false, err
	}
	ret.DisplayName = v

	if mappings.Description != "" {
		v, err = s.env.EvaluateString(ctx, mappings.Description, inputs)
		if err != nil {
			return nil, false, err
		}
		ret.Description = v
	}

	resourceTypes, err := s.fullConfig.GetResourceTypes(ctx)
	if err != nil {
		return nil, false, err
	}
	for _, rt := range mappings.GrantableTo {
		for _, r := range resourceTypes {
			if r.Id == rt {
				ret.GrantableTo = append(ret.GrantableTo, r)
			}
		}
	}

	// TODO(jirwin): Should entitlement slugs be required?
	if mappings.Slug == "" {
		return nil, false, fmt.Errorf("entitlements mapping slug is required")
	}
	v, err = s.env.EvaluateString(ctx, mappings.Slug, inputs)
	if err != nil {
		return nil, false, err
	}
	ret.Slug = v

	var purpose string
	if mappings.Purpose != "" {
		purpose, err = s.env.EvaluateString(ctx, mappings.Purpose, inputs)
		if err != nil {
			return nil, false, err
		}
	}
	switch purpose {
	case "assignment":
		ret.Purpose = v2.Entitlement_PURPOSE_VALUE_ASSIGNMENT
	case "permission":
		ret.Purpose = v2.Entitlement_PURPOSE_VALUE_PERMISSION
	default:
		ret.Purpose = v2.Entitlement_PURPOSE_VALUE_UNSPECIFIED
	}

	annos := annotations.Annotations(ret.Annotations)
	if mappings.Immutable {
		annos.Update(&v2.EntitlementImmutable{})
	}
	ret.Annotations = annos

	return ret, true, nil
}
