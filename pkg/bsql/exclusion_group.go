package bsql

import (
	"context"
	"fmt"
	"strconv"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"google.golang.org/protobuf/types/known/anypb"
)

// buildExclusionGroupAny evaluates the three CEL expressions on an
// ExclusionGroupMapping and produces a protowire-encoded Any carrying the
// EntitlementExclusionGroup message. Returns nil when cfg is nil.
func (s *SQLSyncer) buildExclusionGroupAny(ctx context.Context, cfg *ExclusionGroupMapping, inputs map[string]any) (*anypb.Any, error) {
	if cfg == nil {
		return nil, nil
	}

	if cfg.Id == "" {
		return nil, fmt.Errorf("exclusion_group.id is required")
	}
	groupID, err := s.env.EvaluateString(ctx, cfg.Id, inputs)
	if err != nil {
		return nil, fmt.Errorf("exclusion_group.id evaluation failed: %w", err)
	}
	if groupID == "" {
		return nil, fmt.Errorf("exclusion_group.id evaluated to empty string")
	}

	group := &v2.EntitlementExclusionGroup{}

	group.SetExclusionGroupId(groupID)

	if cfg.Order != "" {
		orderStr, err := s.env.EvaluateString(ctx, cfg.Order, inputs)
		if err != nil {
			return nil, fmt.Errorf("exclusion_group.order evaluation failed: %w", err)
		}
		if orderStr != "" {
			parsed, err := strconv.ParseUint(orderStr, 10, 32)
			if err != nil {
				return nil, fmt.Errorf("exclusion_group.order must be a non-negative integer, got %q: %w", orderStr, err)
			}
			group.SetOrder(uint32(parsed))
		}
	}

	var isDefault bool
	if cfg.IsDefault != "" {
		isDefault, err = s.env.EvaluateBool(ctx, cfg.IsDefault, inputs)
		if err != nil {
			return nil, fmt.Errorf("exclusion_group.is_default evaluation failed: %w", err)
		}
	}

	group.SetIsDefault(isDefault)

	return anypb.New(group)
}
