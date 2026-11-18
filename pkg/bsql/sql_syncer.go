package bsql

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	"github.com/conductorone/baton-sql/pkg/bcel"
	"github.com/conductorone/baton-sql/pkg/database"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	userTraitType   = "user"
	appTraitType    = "app"
	groupTraitType  = "group"
	roleTraitType   = "role"
	secretTraitType = "secret"
	agentTraitType  = "agent"

	scopeCluster        = "cluster"
	dbIterPageStateType = "db-iter"
	rowColDatabase      = "database"
)

var _ connectorbuilder.ResourceTargetedSyncer = (*SQLSyncer)(nil)

// SQLSyncer mutates db / currentDBName between query passes during multi-database
// iteration. Safe because the SDK calls List/Entitlements/Grants serially per syncer.
type SQLSyncer struct {
	resourceType  *v2.ResourceType
	db            *sql.DB
	currentDBName string
	dbs           map[string]*sql.DB
	dbNames       []string
	primaryDBName string
	dbEngine      database.DbEngine
	config        ResourceType
	env           *bcel.Env
	fullConfig    Config
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
		return nil, nil, status.Errorf(codes.Unimplemented, "baton-sql: get not configured for resource type %s", resourceId.GetResourceType())
	}

	vars, err := s.PrepareQueryVars(ctx, nil, s.config.Get.Vars)
	if err != nil {
		return nil, nil, fmt.Errorf("baton-sql: failed to prepare vars for get query: %w", err)
	}
	vars[resourceIdKey] = resourceId.GetResource()

	var result *v2.Resource
	// Get runs against the primary database only, even when multiple databases are configured.
	_, err = s.iterateDBs(ctx, scopeCluster, &pagination.Token{}, func(ctx context.Context, _ string, innerToken *pagination.Token) (string, error) {
		return s.runQuery(ctx, s.db, innerToken, s.config.Get.Query, nil, vars, func(ctx context.Context, row map[string]interface{}) (bool, error) {
			r, err := s.mapResource(ctx, row)
			if err != nil {
				return false, err
			}
			result = r
			return false, nil // stop after first row
		})
	})
	if err != nil {
		return nil, nil, fmt.Errorf("baton-sql: failed to execute get query for %s/%s: %w", resourceId.GetResourceType(), resourceId.GetResource(), err)
	}
	if result == nil {
		return nil, nil, status.Errorf(codes.NotFound, "baton-sql: resource %s/%s not found", resourceId.GetResourceType(), resourceId.GetResource())
	}
	return result, nil, nil
}

// setCurrentDB errors on unknown names rather than silently falling back; the previous
// fallback could mask a programming bug by routing queries to an unintended database.
func (s *SQLSyncer) setCurrentDB(name string) error {
	db, ok := s.dbs[name]
	if !ok {
		return fmt.Errorf("setCurrentDB: unknown database %q (configured: %v)", name, s.dbNames)
	}
	s.db = db
	s.currentDBName = name
	return nil
}

func sortedDBNames(dbs map[string]*sql.DB) []string {
	names := make([]string, 0, len(dbs))
	for name := range dbs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c Config) GetSQLSyncers(ctx context.Context, dbs map[string]*sql.DB, dbEngine database.DbEngine, celEnv *bcel.Env) ([]connectorbuilder.ResourceSyncer, error) {
	if len(dbs) == 0 {
		return nil, fmt.Errorf("GetSQLSyncers: no database handles provided")
	}

	dbNames := sortedDBNames(dbs)
	primaryDBName := dbNames[0]
	primaryDB := dbs[primaryDBName]

	var ret []connectorbuilder.ResourceSyncer
	for rtID, rtConfig := range c.ResourceTypes {
		rt, err := c.GetResourceType(ctx, rtID)
		if err != nil {
			return nil, err
		}

		var rv connectorbuilder.ResourceSyncer

		// If the resource type has account provisioning, use for account provisioning
		if rtConfig.AccountProvisioning != nil {
			rv = newUserSyncer(rt, rtConfig, dbs, dbNames, primaryDBName, dbEngine, celEnv, c)
		} else {
			rv = &SQLSyncer{
				resourceType:  rt,
				config:        rtConfig,
				db:            primaryDB,
				currentDBName: primaryDBName,
				dbs:           dbs,
				dbNames:       dbNames,
				primaryDBName: primaryDBName,
				dbEngine:      dbEngine,
				env:           celEnv,
				fullConfig:    c,
			}
		}
		ret = append(ret, rv)
	}

	return ret, nil
}

func NewActionSyncer(ctx context.Context, dbs map[string]*sql.DB, dbEngine database.DbEngine, celEnv *bcel.Env, fullConfig Config) (*SQLSyncer, error) {
	if len(dbs) == 0 {
		return nil, fmt.Errorf("NewActionSyncer: no database handles provided")
	}
	dbNames := sortedDBNames(dbs)
	primaryDBName := dbNames[0]
	return &SQLSyncer{
		resourceType:  nil,
		db:            dbs[primaryDBName],
		currentDBName: primaryDBName,
		dbs:           dbs,
		dbNames:       dbNames,
		primaryDBName: primaryDBName,
		dbEngine:      dbEngine,
		env:           celEnv,
		fullConfig:    fullConfig,
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

	if s.fullConfig.EventFeed != nil {
		if err := s.validateInternal(ctx, s.fullConfig.EventFeed); err != nil {
			return fmt.Errorf("validation error for event_feed: %w", err)
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

	if s.config.EventFeed != nil {
		if s.config.Get == nil {
			return s.validateFormatErr("event_feed", fmt.Errorf("get query is required when event_feed is configured"))
		}
		if err := s.validateInternal(ctx, s.config.EventFeed); err != nil {
			return s.validateFormatErr("event_feed", err)
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
