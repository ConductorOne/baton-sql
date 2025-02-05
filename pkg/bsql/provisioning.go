package bsql

import (
	"context"
	"errors"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sql/pkg/helpers"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
)

// getProvisioningConfig fetches the provisioning config for the given entitlement if it exists.
func (s *SQLSyncer) getProvisioningConfig(ctx context.Context, entitlementID string) (*EntitlementProvisioning, bool) {
	l := ctxzap.Extract(ctx)

	for _, e := range s.config.StaticEntitlements {
		if e.Id != entitlementID {
			continue
		}

		if e.Provisioning != nil {
			l.Info("provisioning is enabled for entitlement", zap.String("entitlement_id", entitlementID))
			return e.Provisioning, true
		}
	}

	// Check dynamic entitlements
	if s.config.Entitlements != nil {
		for _, e := range s.config.Entitlements.Map {
			if e.Provisioning != nil {
				l.Info("provisioning is enabled for entitlement", zap.String("entitlement_id", entitlementID))
				return e.Provisioning, true
			}
		}
	}

	return nil, false
}

func (s *SQLSyncer) Grant(ctx context.Context, principal *v2.Resource, entitlement *v2.Entitlement) (annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)

	l.Debug("granting entitlement", zap.String("entitlement_id", entitlement.GetId()))

	_, _, entitlementID, err := helpers.SplitEntitlementID(entitlement)
	if err != nil {
		return nil, err
	}

	provisioningConfig, ok := s.getProvisioningConfig(ctx, entitlementID)
	if !ok {
		return nil, errors.New("provisioning is not enabled for this connector")
	}

	if provisioningConfig.Grant == nil {
		return nil, errors.New("no grant config found for entitlement")
	}

	if len(provisioningConfig.Grant.Queries) == 0 {
		return nil, errors.New("no grant config found for entitlement")
	}

	provisioningVars, err := s.prepareProvisioningVars(ctx, provisioningConfig.Vars, principal, entitlement)
	if err != nil {
		return nil, err
	}

	useTx := true
	if provisioningConfig.Grant.NoTransaction {
		useTx = false
	}
	err = s.runProvisioningQueries(ctx, provisioningConfig.Grant.Queries, provisioningVars, useTx)
	if err != nil {
		return nil, err
	}

	l.Debug(
		"granted entitlement",
		zap.String("principal_id", principal.GetId().GetResource()),
		zap.String("entitlement_id", entitlement.GetId()),
	)
	return nil, nil
}

func (s *SQLSyncer) Revoke(ctx context.Context, grant *v2.Grant) (annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)

	l.Debug(
		"revoking entitlement",
		zap.String("grant_id", grant.GetId()),
	)

	_, _, entitlementID, err := helpers.SplitEntitlementID(grant.GetEntitlement())
	if err != nil {
		return nil, err
	}

	provisioningConfig, ok := s.getProvisioningConfig(ctx, entitlementID)
	if !ok {
		return nil, errors.New("provisioning is not enabled for this connector")
	}

	if provisioningConfig.Revoke == nil {
		return nil, errors.New("no revoke config found for entitlement")
	}

	if len(provisioningConfig.Revoke.Queries) == 0 {
		return nil, errors.New("no revoke config found for entitlement")
	}

	provisioningVars, err := s.prepareProvisioningVars(ctx, provisioningConfig.Vars, grant.GetPrincipal(), grant.GetEntitlement())
	if err != nil {
		return nil, err
	}

	useTx := true
	if provisioningConfig.Revoke.NoTransaction {
		useTx = false
	}

	err = s.runProvisioningQueries(ctx, provisioningConfig.Revoke.Queries, provisioningVars, useTx)
	if err != nil {
		return nil, err
	}

	l.Debug("revoked grant", zap.String("grant_id", grant.GetId()))
	return nil, nil
}

func (s *SQLSyncer) prepareProvisioningVars(ctx context.Context, vars map[string]string, principal *v2.Resource, entitlement *v2.Entitlement) (map[string]any, error) {
	if principal == nil {
		return nil, errors.New("principal is required")
	}

	if entitlement == nil {
		return nil, errors.New("entitlement is required")
	}

	ret := make(map[string]any)

	inputs, err := s.env.ProvisioningInputs(principal, entitlement)
	if err != nil {
		return nil, err
	}

	for k, v := range vars {
		out, err := s.env.Evaluate(ctx, v, inputs)
		if err != nil {
			return nil, err
		}
		ret[k] = out
	}

	return ret, nil
}
