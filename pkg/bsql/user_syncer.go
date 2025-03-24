package bsql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/crypto"
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
	l := ctxzap.Extract(ctx)
	resourceTypeID, accountProvisioning, err := s.fullConfig.ExtractAccountProvisioning()
	if err != nil {
		if errors.Is(err, ErrNoAccountProvisioningDefined) {
			return nil, nil, nil, nil
		}
		return nil, nil, nil, err
	}

	// check credentialOptions here (return random-password if specified)
	// use config from yaml for random password generation

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

	// generate password
	plainTextPassword, err := crypto.GeneratePassword(credentialOptions)
	if err != nil {
		return nil, nil, nil, err
	}
	ptd := &v2.PlaintextData{
		Name:  "password",
		Bytes: []byte(plainTextPassword),
	}

	// not sure if this is a right way to pass in password
	//inputs["password"] = []byte(plainTextPassword)

	provisioningVars, err := s.env.AccountProvisioningInputs(inputs)
	if err != nil {
		return nil, nil, nil, err
	}

	useTx := true
	if accountProvisioning.Create.NoTransaction {
		useTx = false
	}

	err = s.SQLSyncer.runProvisioningQueries(ctx, accountProvisioning.Create.Queries, provisioningVars, useTx)
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

	return car, []*v2.PlaintextData{ptd}, nil, nil
}
