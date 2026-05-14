package bsql

import (
	"context"
	"fmt"
	"strconv"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/types/known/anypb"
)

// EntitlementExclusionGroupTypeURL is the Any type_url for the upstream
// c1.connector.v2.EntitlementExclusionGroup message. Once the baton-sdk
// branch lands and is vendored, callers can switch to
// annotations.Update(&v2.EntitlementExclusionGroup{...}) and this hand-rolled
// encoder can be deleted.
const EntitlementExclusionGroupTypeURL = "type.googleapis.com/c1.connector.v2.EntitlementExclusionGroup"

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

	var order uint32
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
			order = uint32(parsed)
		}
	}

	var isDefault bool
	if cfg.IsDefault != "" {
		isDefault, err = s.env.EvaluateBool(ctx, cfg.IsDefault, inputs)
		if err != nil {
			return nil, fmt.Errorf("exclusion_group.is_default evaluation failed: %w", err)
		}
	}

	var buf []byte
	// field 1: exclusion_group_id (string)
	buf = protowire.AppendTag(buf, 1, protowire.BytesType)
	buf = protowire.AppendString(buf, groupID)
	// field 2: order (uint32) — omit when zero to match proto3 defaults
	if order != 0 {
		buf = protowire.AppendTag(buf, 2, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(order))
	}
	// field 3: is_default (bool) — omit when false
	if isDefault {
		buf = protowire.AppendTag(buf, 3, protowire.VarintType)
		buf = protowire.AppendVarint(buf, protowire.EncodeBool(true))
	}

	return &anypb.Any{
		TypeUrl: EntitlementExclusionGroupTypeURL,
		Value:   buf,
	}, nil
}
