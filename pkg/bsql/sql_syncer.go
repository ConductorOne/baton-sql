package bsql

import (
	"context"
	"database/sql"

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
