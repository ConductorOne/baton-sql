package bsql

import (
	"context"
	"database/sql"
	"fmt"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sql/pkg/bcel"
	"github.com/conductorone/baton-sql/pkg/database"
)

const (
	userTraitType  = "user"
	appTraitType   = "app"
	groupTraitType = "group"
	roleTraitType  = "role"
)

type SQLSyncer struct {
	resourceType *v2.ResourceType
	db           *sql.DB
	dbEngine     database.DbEngine
	config       ResourceType
	env          *bcel.Env
	fullConfig   Config
}

func (s *SQLSyncer) ResourceType(ctx context.Context) *v2.ResourceType {
	return s.resourceType
}

func (c Config) GetSQLSyncers(ctx context.Context, db *sql.DB, dbEngine database.DbEngine, celEnv *bcel.Env) ([]connectorbuilder.ResourceSyncer, error) {
	var ret []connectorbuilder.ResourceSyncer
	for rtID, rtConfig := range c.ResourceTypes {
		rt, err := c.GetResourceType(ctx, rtID)
		if err != nil {
			return nil, err
		}

		var rv connectorbuilder.ResourceSyncer

		// If the resource type has account provisioning, use for account provisioning
		if rtConfig.AccountProvisioning != nil {
			rv = newUserSyncer(rt, rtConfig, db, dbEngine, celEnv, c)
		} else {
			rv = &SQLSyncer{
				resourceType: rt,
				config:       rtConfig,
				db:           db,
				dbEngine:     dbEngine,
				env:          celEnv,
				fullConfig:   c,
			}
		}
		ret = append(ret, rv)
	}

	return ret, nil
}

func NewActionSyncer(ctx context.Context, db *sql.DB, dbEngine database.DbEngine, celEnv *bcel.Env, fullConfig Config) (*SQLSyncer, error) {
	return &SQLSyncer{
		resourceType: nil,
		db:           db,
		dbEngine:     dbEngine,
		env:          celEnv,
		fullConfig:   fullConfig,
	}, nil
}

func (s *SQLSyncer) validateInternal(ctx context.Context, anyV any) error {
	if anyV == nil {
		return nil
	}

	if v, ok := anyV.(staticValidator); ok {
		err := v.StaticValidate(ctx, s)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *SQLSyncer) validateFormatErr(field string, err error) error {
	rsTypeId := s.resourceType.Id

	return fmt.Errorf("validation error for resource type %q, field %q: %w", rsTypeId, field, err)
}

func (s *SQLSyncer) Validate(ctx context.Context) error {
	if s.fullConfig.Actions != nil {
		for key, action := range s.fullConfig.Actions {
			err := s.validateInternal(ctx, &action)
			if err != nil {
				return s.validateFormatErr(fmt.Sprintf("Action[%s]", key), err)
			}
		}
	}

	if err := s.validateInternal(ctx, s.config.List); err != nil {
		return s.validateFormatErr("list", err)
	}

	if s.config.Entitlements != nil {
		if err := s.validateInternal(ctx, s.config.Entitlements); err != nil {
			return s.validateFormatErr("entitlements", err)
		}
	}

	if s.config.StaticEntitlements != nil {
		if err := s.validateInternal(ctx, s.config.StaticEntitlements); err != nil {
			return s.validateFormatErr("static_entitlements", err)
		}
	}

	if s.config.Grants != nil {
		if err := s.validateInternal(ctx, s.config.Grants); err != nil {
			return s.validateFormatErr("grants", err)
		}
	}

	if s.config.AccountProvisioning != nil {
		if err := s.validateInternal(ctx, s.config.AccountProvisioning); err != nil {
			return s.validateFormatErr("account_provisioning", err)
		}
	}

	if s.config.CredentialRotation != nil {
		if err := s.validateInternal(ctx, s.config.CredentialRotation); err != nil {
			return s.validateFormatErr("credential_rotation", err)
		}
	}

	return nil
}
