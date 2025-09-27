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

var _ connectorbuilder.AccountManager = &userSyncer{}
var _ connectorbuilder.CredentialManager = &userSyncer{}

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

func optionsFromCredentials(credentials *AccountCredentials) ([]v2.CapabilityDetailCredentialOption, []v2.CapabilityDetailCredentialOption, error) {
	if credentials == nil {
		return nil, nil, errors.New("no credential options defined")
	}

	var supportedCredentials []v2.CapabilityDetailCredentialOption
	var preferredCredentialOption []v2.CapabilityDetailCredentialOption
	if credentials.NoPassword != nil {
		supportedCredentials = append(supportedCredentials, v2.CapabilityDetailCredentialOption_CAPABILITY_DETAIL_CREDENTIAL_OPTION_NO_PASSWORD)
		if credentials.NoPassword.Preferred {
			preferredCredentialOption = append(preferredCredentialOption, v2.CapabilityDetailCredentialOption_CAPABILITY_DETAIL_CREDENTIAL_OPTION_NO_PASSWORD)
		}
	}
	if credentials.RandomPassword != nil {
		supportedCredentials = append(supportedCredentials, v2.CapabilityDetailCredentialOption_CAPABILITY_DETAIL_CREDENTIAL_OPTION_RANDOM_PASSWORD)
		if credentials.RandomPassword.Preferred {
			preferredCredentialOption = append(preferredCredentialOption, v2.CapabilityDetailCredentialOption_CAPABILITY_DETAIL_CREDENTIAL_OPTION_RANDOM_PASSWORD)
		}
	}
	if credentials.EncryptedPassword != nil {
		supportedCredentials = append(supportedCredentials, v2.CapabilityDetailCredentialOption_CAPABILITY_DETAIL_CREDENTIAL_OPTION_ENCRYPTED_PASSWORD)
		if credentials.EncryptedPassword.Preferred {
			preferredCredentialOption = append(preferredCredentialOption, v2.CapabilityDetailCredentialOption_CAPABILITY_DETAIL_CREDENTIAL_OPTION_ENCRYPTED_PASSWORD)
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

	return supportedCredentials, preferredCredentialOption, nil
}

func (s *userSyncer) CreateAccountCapabilityDetails(ctx context.Context) (*v2.CredentialDetailsAccountProvisioning, annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)

	resourceTypeID, accountProvisioning, err := s.extractAndValidateProvisioning()
	if err != nil {
		return nil, nil, err
	}

	l.Debug("account provisioning is enabled", zap.String("resource_type_id", resourceTypeID))

	if accountProvisioning == nil {
		return nil, nil, errors.New("no account provisioning defined")
	}

	supportedCredentials, preferredCredentialOption, err := optionsFromCredentials(accountProvisioning.Credentials)
	if err != nil {
		return nil, nil, err
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
	credentialOptions *v2.LocalCredentialOptions,
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
	queryInputs, plaintextDataList, err := s.prepareQueryInputs(ctx, provisioningConfig, accountInfo, credentialOptions)
	if err != nil {
		return nil, nil, nil, err
	}

	// Execute account creation queries
	useTransaction := !provisioningConfig.Create.NoTransaction
	if err := s.runProvisioningQueries(ctx, provisioningConfig.Create.Queries, queryInputs, useTransaction); err != nil {
		return nil, nil, nil, err
	}

	// Validate the created account
	accountResource, err := s.validateAccount(ctx, provisioningConfig, queryInputs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to validate created account: %w", err)
	}

	response := &v2.CreateAccountResponse_SuccessResult{
		Resource: accountResource,
	}

	return response, plaintextDataList, nil, nil
}

func (s *userSyncer) Rotate(ctx context.Context, resourceId *v2.ResourceId, credentialOptions *v2.LocalCredentialOptions) ([]*v2.PlaintextData, annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)

	// Extract and validate account provisioning configuration
	resourceTypeID, rotationConfig, err := s.extractAndValidateCredentialRotation()
	if err != nil {
		return nil, nil, err
	}

	l.Debug("rotating credential", zap.String("resource_type_id", resourceTypeID))

	queryInputs := make(map[string]any)
	queryInputs["resource_id"] = resourceId.Resource
	credentials := make(map[string]any)
	var plaintextDataList []*v2.PlaintextData
	password, err := generatePassword(ctx, credentialOptions)
	if err != nil {
		return nil, nil, err
	}
	if password != nil {
		queryInputs["password"] = *password
		credentials["password"] = *password
		// Create plaintext data for return
		passwordData := &v2.PlaintextData{
			Name:  "password",
			Bytes: []byte(*password),
		}
		plaintextDataList = append(plaintextDataList, passwordData)
	}

	// Execute account creation queries
	useTransaction := !rotationConfig.Update.NoTransaction
	if err := s.runProvisioningQueries(ctx, rotationConfig.Update.Queries, queryInputs, useTransaction); err != nil {
		return nil, nil, err
	}

	return plaintextDataList, nil, nil
}

func (s *userSyncer) RotateCapabilityDetails(ctx context.Context) (*v2.CredentialDetailsCredentialRotation, annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)
	resourceTypeID, rotationConfig, err := s.fullConfig.ExtractCredentialRotation()
	if err != nil {
		if errors.Is(err, ErrNoCredentialRotationDefined) {
			return nil, nil, nil
		}

		return nil, nil, err
	}

	l.Debug("credential rotation is enabled", zap.String("resource_type_id", resourceTypeID))

	if rotationConfig == nil {
		return nil, nil, errors.New("no credential rotation configuration defined")
	}

	supportedCredentials, preferredCredentialOption, err := optionsFromCredentials(rotationConfig.Credentials)
	if err != nil {
		return nil, nil, err
	}

	return &v2.CredentialDetailsCredentialRotation{
		SupportedCredentialOptions: supportedCredentials,
		PreferredCredentialOption:  preferredCredentialOption[0],
	}, nil, nil
}
