package bsql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sql/pkg/bcel"
	"github.com/conductorone/baton-sql/pkg/database"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
)

type userSyncer struct {
	*SQLSyncer
}

func newUserSyncer(rt *v2.ResourceType, rtConfig ResourceType, db *sql.DB, dbEngine database.DbEngine, celEnv *bcel.Env, fullConfig Config) *userSyncer {
	sqlSyncer := &SQLSyncer{
		resourceType: rt,
		config:       rtConfig,
		db:           db,
		dbEngine:     dbEngine,
		env:          celEnv,
		fullConfig:   fullConfig,
	}

	return &userSyncer{
		SQLSyncer: sqlSyncer,
	}
}

func (s *userSyncer) CreateAccountCapabilityDetails(ctx context.Context) (*v2.CredentialDetailsAccountProvisioning, annotations.Annotations, error) {
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

// CreateAccount creates a new user account in the database with optional credential generation.
// It validates inputs, generates credentials if required, executes provisioning queries,
// and validates the created account.
func (s *userSyncer) CreateAccount(
	ctx context.Context,
	accountInfo *v2.AccountInfo,
	credentialOptions *v2.CredentialOptions,
) (
	connectorbuilder.CreateAccountResponse,
	[]*v2.PlaintextData,
	annotations.Annotations,
	error,
) {
	logger := ctxzap.Extract(ctx)

	// Extract and validate account provisioning configuration
	resourceTypeID, provisioningConfig, err := s.extractAndValidateProvisioning()
	if err != nil {
		return nil, nil, nil, err
	}

	logger.Debug("creating account", zap.String("resource_type_id", resourceTypeID))

	// Validate required input parameters
	if err := s.validateAccountInfo(accountInfo); err != nil {
		return nil, nil, nil, err
	}

	// Prepare all query inputs in one step
	queryInputs, plaintextDataList, err := s.prepareAllQueryInputs(provisioningConfig, accountInfo, credentialOptions)
	if err != nil {
		return nil, nil, nil, err
	}

	// Execute account creation queries
	useTransaction := !provisioningConfig.Create.NoTransaction
	if err := s.runProvisioningQueries(ctx, provisioningConfig.Create.Queries, queryInputs, useTransaction); err != nil {
		return nil, nil, nil, err
	}

	// Validate the created account
	accountResource, err := s.validateCreatedAccount(ctx, provisioningConfig, queryInputs)
	if err != nil {
		return nil, nil, nil, err
	}

	response := &v2.CreateAccountResponse_SuccessResult{
		Resource: accountResource,
	}

	return response, plaintextDataList, nil, nil
}

// extractAndValidateProvisioning extracts and validates the account provisioning configuration.
func (s *userSyncer) extractAndValidateProvisioning() (string, *AccountProvisioning, error) {
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

// validateAccountInfo validates that the required account information is provided.
func (s *userSyncer) validateAccountInfo(accountInfo *v2.AccountInfo) error {
	if accountInfo == nil {
		return errors.New("account info is required")
	}

	if accountInfo.Profile == nil {
		return errors.New("account profile is required")
	}

	return nil
}

// prepareAllQueryInputs prepares all query inputs including schema vars and credentials in one step.
// This eliminates the need for complex merging logic by doing everything together.
func (s *userSyncer) prepareAllQueryInputs(
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

// validateCreatedAccount validates that the account was created successfully.
func (s *userSyncer) validateCreatedAccount(ctx context.Context, provisioningConfig *AccountProvisioning, queryInputs map[string]any) (*v2.Resource, error) {
	accountResource, isValid, err := s.validateAccount(ctx, provisioningConfig, queryInputs)
	if err != nil {
		return nil, fmt.Errorf("failed to validate created account: %w", err)
	}

	if !isValid {
		return nil, errors.New("account validation failed after creation")
	}

	return accountResource, nil
}
