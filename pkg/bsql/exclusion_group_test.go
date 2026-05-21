package bsql

import (
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sql/pkg/bcel"
	"github.com/stretchr/testify/require"
)

func newTestSyncerWithEnv(t *testing.T) *SQLSyncer {
	t.Helper()
	env, err := bcel.NewEnv(t.Context())
	require.NoError(t, err)
	return &SQLSyncer{env: env}
}

func TestBuildExclusionGroupAny(t *testing.T) {
	ctx := t.Context()

	t.Run("nil config returns nil any", func(t *testing.T) {
		s := newTestSyncerWithEnv(t)
		got, err := s.buildExclusionGroupAny(ctx, nil, map[string]any{})
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("missing id is rejected", func(t *testing.T) {
		s := newTestSyncerWithEnv(t)
		_, err := s.buildExclusionGroupAny(ctx, &ExclusionGroupMapping{}, map[string]any{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "id is required")
	})

	t.Run("id that evaluates to empty string is rejected", func(t *testing.T) {
		s := newTestSyncerWithEnv(t)
		_, err := s.buildExclusionGroupAny(ctx, &ExclusionGroupMapping{Id: "''"}, map[string]any{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "evaluated to empty")
	})

	t.Run("id-only mapping produces packed annotation", func(t *testing.T) {
		s := newTestSyncerWithEnv(t)
		anyv, err := s.buildExclusionGroupAny(ctx, &ExclusionGroupMapping{Id: "'license'"}, map[string]any{})
		require.NoError(t, err)
		require.NotNil(t, anyv)

		got := &v2.EntitlementExclusionGroup{}
		require.NoError(t, anyv.UnmarshalTo(got))
		require.Equal(t, "license", got.GetExclusionGroupId())
		require.Equal(t, uint32(0), got.GetOrder())
		require.False(t, got.GetIsDefault())
	})

	t.Run("order and is_default are evaluated", func(t *testing.T) {
		s := newTestSyncerWithEnv(t)
		mapping := &ExclusionGroupMapping{
			Id:                "'license'",
			Order:             "'2'",
			IsDefault:         "true",
			IsScopeToResource: "true",
		}
		anyv, err := s.buildExclusionGroupAny(ctx, mapping, map[string]any{})
		require.NoError(t, err)

		got := &v2.EntitlementExclusionGroup{}
		require.NoError(t, anyv.UnmarshalTo(got))
		require.Equal(t, "license", got.GetExclusionGroupId())
		require.Equal(t, uint32(2), got.GetOrder())
		require.True(t, got.GetIsDefault())
		require.True(t, got.GetScopeToResource())
	})

	t.Run("order reads from row inputs", func(t *testing.T) {
		s := newTestSyncerWithEnv(t)
		mapping := &ExclusionGroupMapping{
			Id:    "string(cols.tier)",
			Order: "string(cols.order)",
		}
		inputs := s.env.SyncInputs(map[string]any{
			"tier":  "premium",
			"order": int64(5),
		})
		anyv, err := s.buildExclusionGroupAny(ctx, mapping, inputs)
		require.NoError(t, err)

		got := &v2.EntitlementExclusionGroup{}
		require.NoError(t, anyv.UnmarshalTo(got))
		require.Equal(t, "premium", got.GetExclusionGroupId())
		require.Equal(t, uint32(5), got.GetOrder())
	})

	t.Run("non-integer order is rejected", func(t *testing.T) {
		s := newTestSyncerWithEnv(t)
		_, err := s.buildExclusionGroupAny(ctx, &ExclusionGroupMapping{
			Id:    "'g'",
			Order: "'not-a-number'",
		}, map[string]any{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "non-negative integer")
	})

	t.Run("empty order string is allowed and leaves order at zero", func(t *testing.T) {
		s := newTestSyncerWithEnv(t)
		anyv, err := s.buildExclusionGroupAny(ctx, &ExclusionGroupMapping{
			Id:    "'g'",
			Order: "''",
		}, map[string]any{})
		require.NoError(t, err)

		got := &v2.EntitlementExclusionGroup{}
		require.NoError(t, anyv.UnmarshalTo(got))
		require.Equal(t, uint32(0), got.GetOrder())
	})

	t.Run("invalid CEL in id surfaces the eval error", func(t *testing.T) {
		s := newTestSyncerWithEnv(t)
		_, err := s.buildExclusionGroupAny(ctx, &ExclusionGroupMapping{Id: "this is not cel"}, map[string]any{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "id evaluation failed")
	})
}
