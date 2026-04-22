package bsql

import (
	"context"
	"database/sql"
	"fmt"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/pagination"
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

// ResourceTypeID returns the resource type ID for this syncer.
func (s *SQLSyncer) ResourceTypeID() string {
	if s.resourceType == nil {
		return ""
	}
	return s.resourceType.Id
}

// Get implements ResourceTargetedSyncer, fetching a single resource by its ID.
func (s *SQLSyncer) Get(ctx context.Context, resourceId *v2.ResourceId, parentResourceId *v2.ResourceId) (*v2.Resource, annotations.Annotations, error) {
	if s.config.Get == nil {
		return nil, nil, fmt.Errorf("baton-sql: get not configured for resource type %s", resourceId.GetResourceType())
	}

	vars, err := s.PrepareQueryVars(ctx, nil, s.config.Get.Vars)
	if err != nil {
		return nil, nil, fmt.Errorf("baton-sql: failed to prepare vars for get query: %w", err)
	}
	vars[idKey] = resourceId.GetResource()

	var result *v2.Resource
	_, err = s.runQuery(ctx, &pagination.Token{}, s.config.Get.Query, nil, vars, func(ctx context.Context, row map[string]interface{}) (bool, error) {
		r, err := s.mapResource(ctx, row)
		if err != nil {
			return false, err
		}
		result = r
		return false, nil // stop after first row
	})
	if err != nil {
		return nil, nil, fmt.Errorf("baton-sql: failed to execute get query for %s/%s: %w", resourceId.GetResourceType(), resourceId.GetResource(), err)
	}
	if result == nil {
		return nil, nil, fmt.Errorf("baton-sql: resource %s/%s not found", resourceId.GetResourceType(), resourceId.GetResource())
	}
	return result, nil, nil
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

func (s *SQLSyncer) validateInternal(ctx context.Context, validator staticValidator) error {
	if validator == nil {
		return nil
	}

	err := validator.staticValidate(ctx, s)
	if err != nil {
		return err
	}

	return nil
}

func (s *SQLSyncer) validateFormatErr(field string, err error) error {
	if s.resourceType == nil {
		return fmt.Errorf("validation error for action config, field %q: %w", field, err)
	}

	return fmt.Errorf("validation error for resource type %q, field %q: %w", s.resourceType.Id, field, err)
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
		for _, entitlement := range s.config.StaticEntitlements {
			if err := s.validateInternal(ctx, entitlement); err != nil {
				return s.validateFormatErr("static_entitlements", err)
			}
		}
	}

	if s.config.Grants != nil {
		for _, grant := range s.config.Grants {
			if err := s.validateInternal(ctx, grant); err != nil {
				return s.validateFormatErr("grants", err)
			}
		}
	}

	if s.config.Get != nil {
		if err := s.validateInternal(ctx, s.config.Get); err != nil {
			return s.validateFormatErr("get", err)
		}
	}

	if s.config.IncrementalSync != nil {
		if s.config.Get == nil {
			return s.validateFormatErr("incremental_sync", fmt.Errorf("get query is required when incremental_sync is configured"))
		}
		if err := s.validateInternal(ctx, s.config.IncrementalSync); err != nil {
			return s.validateFormatErr("incremental_sync", err)
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
