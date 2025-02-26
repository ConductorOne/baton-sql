package bsql

import (
	"context"
	"errors"
	"fmt"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
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

func (s *SQLSyncer) CreateAccountCapabilityDetails(ctx context.Context) (*v2.CredentialDetailsAccountProvisioning, annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)
	resourceTypeID, accountProvisioning, err := s.fullConfig.ExtractAccountProvisioning()
	if err != nil {
		if errors.Is(err, ErrNoAccountProvisioningDefined) {
			return nil, nil, nil
		}

		return nil, nil, err
	}

	l.Debug("account provisioning is enabled", zap.String("resource_type_id", resourceTypeID))

	if accountProvisioning == nil {
		return nil, nil, errors.New("no account provisioning defined")
	}

	if accountProvisioning.Credentials == nil {
		return nil, nil, errors.New("no credential options defined")
	}

	var supportedCredentials []v2.CapabilityDetailCredentialOption
	var preferredCredentialOption []v2.CapabilityDetailCredentialOption

	if accountProvisioning.Credentials.NoPassword != nil {
		supportedCredentials = append(supportedCredentials, v2.CapabilityDetailCredentialOption_CAPABILITY_DETAIL_CREDENTIAL_OPTION_NO_PASSWORD)
		if accountProvisioning.Credentials.NoPassword.Preferred {
			preferredCredentialOption = append(preferredCredentialOption, v2.CapabilityDetailCredentialOption_CAPABILITY_DETAIL_CREDENTIAL_OPTION_NO_PASSWORD)
		}
	}

	if accountProvisioning.Credentials.RandomPassword != nil {
		supportedCredentials = append(supportedCredentials, v2.CapabilityDetailCredentialOption_CAPABILITY_DETAIL_CREDENTIAL_OPTION_RANDOM_PASSWORD)
		if accountProvisioning.Credentials.RandomPassword.Preferred {
			preferredCredentialOption = append(preferredCredentialOption, v2.CapabilityDetailCredentialOption_CAPABILITY_DETAIL_CREDENTIAL_OPTION_RANDOM_PASSWORD)
		}
	}

	if len(supportedCredentials) == 0 {
		return nil, nil, nil
	}

	if len(preferredCredentialOption) > 1 {
		return nil, nil, errors.New("multiple preferred credential options are not supported")
	}

	if len(preferredCredentialOption) == 0 {
		preferredCredentialOption = []v2.CapabilityDetailCredentialOption{supportedCredentials[0]}
	}

	return &v2.CredentialDetailsAccountProvisioning{
		SupportedCredentialOptions: supportedCredentials,
		PreferredCredentialOption:  preferredCredentialOption[0],
	}, nil, nil
}

func (s *SQLSyncer) validateAccount(ctx context.Context, accountProvisioning *AccountProvisioning, inputs map[string]any) (*v2.Resource, bool, error) {
	if accountProvisioning.Validate == nil {
		return nil, false, fmt.Errorf("validation configuration is not defined for account provisioning")
	}

	if accountProvisioning.Validate.Query == "" {
		return nil, false, fmt.Errorf("validation query is not defined for account provisioning")
	}

	queryVars, err := s.prepareQueryVars(ctx, inputs, accountProvisioning.Validate.Vars)
	if err != nil {
		return nil, false, err
	}

	var ret *v2.Resource
	_, err = s.runQuery(ctx, nil, accountProvisioning.Validate.Query, nil, queryVars, func(ctx context.Context, rowMap map[string]any) (bool, error) {
		r, err := s.mapResource(ctx, rowMap)
		if err != nil {
			return false, err
		}

		ret = r
		return false, nil

	})
	if err != nil {
		return nil, false, err
	}

	if ret == nil {
		return nil, false, fmt.Errorf("unable to find resource for account provisioning")
	}

	return ret, true, nil
}

func (s *SQLSyncer) prepareSchemaVars(ctx context.Context, accountProvisioning *AccountProvisioning, accountInfo *v2.AccountInfo) (map[string]any, error) {
	inputs := make(map[string]any)

	for _, field := range accountProvisioning.Schema {
		val, ok := accountInfo.Profile.Fields[field.Name]
		if !ok {
			continue
		}

		switch field.Type {
		case "string":
			if strVal := val.GetStringValue(); strVal != "" {
				inputs[field.Name] = strVal
			}

		case "string_list":
			if listVal := val.GetListValue(); listVal != nil {
				var strList []string
				for _, v := range listVal.Values {
					if str := v.GetStringValue(); str != "" {
						strList = append(strList, str)
					}
				}
				inputs[field.Name] = strList
			}

		case "boolean":
			inputs[field.Name] = val.GetBoolValue()

		case "int":
			if numVal := val.GetNumberValue(); numVal != 0 {
				inputs[field.Name] = int(numVal)
			}

		case "map":
			if structVal := val.GetStructValue(); structVal != nil {
				inputs[field.Name] = structVal.AsMap()
			}
		}
	}

	return inputs, nil
}

func (s *SQLSyncer) CreateAccount(ctx context.Context, accountInfo *v2.AccountInfo, credentialOptions *v2.CredentialOptions) (connectorbuilder.CreateAccountResponse, []v2.PlaintextData, annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)
	resourceTypeID, accountProvisioning, err := s.fullConfig.ExtractAccountProvisioning()
	if err != nil {
		if errors.Is(err, ErrNoAccountProvisioningDefined) {
			return nil, nil, nil, nil
		}

		return nil, nil, nil, err
	}

	if accountProvisioning == nil {
		return nil, nil, nil, errors.New("no account provisioning defined")
	}

	l.Debug("creating account", zap.String("resource_type_id", resourceTypeID))

	// TODO: Implement account creation
	// 1. Validate inputs
	// 2. Read schema
	// 3. Parse account info inputs using schema
	// 4. Prepare account provisioning vars
	// 5. Run provisioning queries
	// 6. Return account info and credentials

	if accountInfo == nil || accountInfo.Profile == nil {
		return nil, nil, nil, errors.New("account info and profile are required")
	}

	inputs, err := s.prepareSchemaVars(ctx, accountProvisioning, accountInfo)
	if err != nil {
		return nil, nil, nil, err
	}

	provisioningVars, err := s.env.AccountProvisioningInputs(inputs)
	if err != nil {
		return nil, nil, nil, err
	}

	useTx := true
	if accountProvisioning.Create.NoTransaction {
		useTx = false
	}

	err = s.runProvisioningQueries(ctx, accountProvisioning.Create.Queries, provisioningVars, useTx)
	if err != nil {
		return nil, nil, nil, err
	}

	accountResource, ok, err := s.validateAccount(ctx, accountProvisioning, inputs)
	if err != nil {
		return nil, nil, nil, err
	}

	if !ok {
		return nil, nil, nil, fmt.Errorf("post account provisioning validation failed")
	}

	car := &v2.CreateAccountResponse_SuccessResult{
		Resource: accountResource,
	}

	return car, nil, nil, nil
}
