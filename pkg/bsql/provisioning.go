package bsql

import (
	"context"
	"errors"
	"fmt"

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

	useTx := !provisioningConfig.Grant.NoTransaction
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

	useTx := !provisioningConfig.Revoke.NoTransaction
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

func (s *SQLSyncer) validateAccount(ctx context.Context, accountProvisioning *AccountProvisioning, inputs map[string]any) (*v2.Resource, error) {
	if accountProvisioning.Validate == nil {
		return nil, fmt.Errorf("validation configuration is not defined for account provisioning")
	}

	if accountProvisioning.Validate.Query == "" {
		return nil, fmt.Errorf("validation query is not defined for account provisioning")
	}

	queryVars, err := s.prepareQueryVars(ctx, inputs, accountProvisioning.Validate.Vars)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	if ret == nil {
		return nil, fmt.Errorf("unable to find resource for account provisioning")
	}

	return ret, nil
}

// prepareQueryInputs prepares all query inputs including schema vars and credentials in one step.
// This eliminates the need for complex merging logic by doing everything together.
func (s *SQLSyncer) prepareQueryInputs(
	provisioningConfig *AccountProvisioning,
	accountInfo *v2.AccountInfo,
	credentialOptions *v2.CredentialOptions,
) (map[string]any, []*v2.PlaintextData, error) {
	queryInputs := make(map[string]any)
	var plaintextDataList []*v2.PlaintextData

	// 1. Add schema variables (profile data) directly
	schemaVars := make(map[string]any)
	for _, field := range provisioningConfig.Schema {
		if value, exists := accountInfo.Profile.Fields[field.Name]; exists {
			var parsedValue any
			switch field.Type {
			case "string":
				if strValue := value.GetStringValue(); strValue != "" {
					parsedValue = strValue
				}
			case "string_list":
				if listValue := value.GetListValue(); listValue != nil {
					var strList []string
					for _, v := range listValue.Values {
						if strValue := v.GetStringValue(); strValue != "" {
							strList = append(strList, strValue)
						}
					}
					parsedValue = strList
				}
			case "boolean":
				parsedValue = value.GetBoolValue()
			case "int":
				if numValue := value.GetNumberValue(); numValue != 0 {
					parsedValue = int(numValue)
				}
			case "map":
				if structValue := value.GetStructValue(); structValue != nil {
					parsedValue = structValue.AsMap()
				}
			}

			if parsedValue != nil {
				queryInputs[field.Name] = parsedValue
				schemaVars[field.Name] = parsedValue
			}
		}
	}

	// 2. Add credentials if required
	credentials := make(map[string]any)
	if credentialOptions != nil {
		switch credentialOptions.Options.(type) {
		case *v2.CredentialOptions_NoPassword_:
		case *v2.CredentialOptions_RandomPassword_:
			password, err := generateCredentials(credentialOptions)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to generate password: %w", err)
			}

			// Add password to queryInputs and credentials map
			// NOTE: For future credential types (SSO, API keys), consider using only the
			// 'credentials' namespace to avoid conflicts with user-defined schema fields
			queryInputs["password"] = password
			credentials["password"] = password

			// Create plaintext data for return
			passwordData := &v2.PlaintextData{
				Name:  "password",
				Bytes: []byte(password),
			}
			plaintextDataList = append(plaintextDataList, passwordData)
		default:
			return nil, nil, fmt.Errorf("unsupported credential options: %v", credentialOptions)
		}
	}

	// 3. Add namespaced access for advanced CEL expressions
	// Only add namespaces if they don't conflict with user-defined schema fields
	if len(schemaVars) > 0 {
		if _, exists := queryInputs["input"]; !exists {
			queryInputs["input"] = schemaVars
		}
	}
	if len(credentials) > 0 {
		if _, exists := queryInputs["credentials"]; !exists {
			queryInputs["credentials"] = credentials
		}
	}

	return queryInputs, plaintextDataList, nil
}

// validateAccountInfo validates that the required account information is provided.
func (s *SQLSyncer) validateAccountInfo(accountInfo *v2.AccountInfo) error {
	if accountInfo == nil {
		return errors.New("account info is required")
	}

	if accountInfo.Profile == nil {
		return errors.New("account profile is required")
	}

	return nil
}

// extractAndValidateProvisioning extracts and validates the account provisioning configuration.
func (s *SQLSyncer) extractAndValidateProvisioning() (string, *AccountProvisioning, error) {
	resourceTypeID, accountProvisioning, err := s.fullConfig.ExtractAccountProvisioning()
	if err != nil {
		if errors.Is(err, ErrNoAccountProvisioningDefined) {
			return "", nil, nil
		}
		return "", nil, err
	}

	if accountProvisioning == nil {
		return "", nil, errors.New("no account provisioning defined")
	}

	return resourceTypeID, accountProvisioning, nil
}
