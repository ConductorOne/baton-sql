package bsql

import (
	"context"
	"fmt"
	"strconv"

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

func (s *SQLSyncer) staticEntitlements(ctx context.Context, resource *v2.Resource, pToken *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	inputs, err := s.env.BaseInputs(nil)
	if err != nil {
		return nil, "", nil, err
	}

	inputs["resource"] = map[string]string{
		"ID":             resource.Id.Resource,
		"ResourceTypeID": resource.Id.ResourceType,
		"DisplayName":    resource.DisplayName,
	}

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

	var ret []*v2.Entitlement

	q, err := parseQueryOpts(ctx, s.config.Entitlements.Query, qCtx)
	if err != nil {
		return nil, "", nil, err
	}

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, "", nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, "", nil, err
	}

	values := make([]interface{}, len(columns))
	scanArgs := make([]interface{}, len(values))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	for rows.Next() {
		if err := rows.Scan(scanArgs...); err != nil {
			return nil, "", nil, err
		}

		rowMap := make(map[string]interface{})
		for i, colName := range columns {
			rowMap[colName] = values[i]
		}

		r, err := s.mapEntitlement(ctx, resource, rowMap)
		if err != nil {
			return nil, "", nil, err
		}
		r.Resource = resource
		ret = append(ret, r)
	}

	if err := rows.Err(); err != nil {
		return nil, "", nil, err
	}

	return ret, "", nil, nil
}

func (s *SQLSyncer) mapEntitlement(ctx context.Context, resource *v2.Resource, rowMap map[string]any) (*v2.Entitlement, error) {
	ret := &v2.Entitlement{}

	inputs, err := s.env.BaseInputs(rowMap)
	if err != nil {
		return nil, err
	}

	inputs["resource"] = map[string]string{
		"ID":             resource.Id.Resource,
		"ResourceTypeID": resource.Id.ResourceType,
		"DisplayName":    resource.DisplayName,
	}

	mappings := s.config.Entitlements.Map

	if mappings.Id == "" {
		return nil, fmt.Errorf("entitlements mapping id is required")
	}
	v, err := s.env.EvaluateString(ctx, mappings.Id, inputs)
	if err != nil {
		return nil, err
	}
	ret.Id = v

	if mappings.DisplayName == "" {
		return nil, fmt.Errorf("entitlements mapping display_name is required")
	}
	v, err = s.env.EvaluateString(ctx, mappings.DisplayName, inputs)
	if err != nil {
		return nil, err
	}
	ret.DisplayName = v

	if mappings.Description != "" {
		v, err = s.env.EvaluateString(ctx, mappings.Description, inputs)
		if err != nil {
			return nil, err
		}
		ret.Description = v
	}

	resourceTypes, err := s.fullConfig.GetResourceTypes(ctx)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("entitlements mapping slug is required")
	}
	v, err = s.env.EvaluateString(ctx, mappings.Slug, inputs)
	if err != nil {
		return nil, err
	}
	ret.Slug = v

	var purpose string
	if mappings.Purpose != "" {
		purpose, err = s.env.EvaluateString(ctx, mappings.Purpose, inputs)
		if err != nil {
			return nil, err
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

	return ret, nil
}
