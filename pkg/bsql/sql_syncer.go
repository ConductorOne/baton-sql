package bsql

import (
	"context"
	"database/sql"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sql/pkg/bcel"
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
	dbType       string
	config       ResourceType
	env          *bcel.Env
	fullConfig   Config
}

func (s *SQLSyncer) ResourceType(ctx context.Context) *v2.ResourceType {
	return s.resourceType
}

func (c Config) GetSQLSyncers(ctx context.Context, db *sql.DB, dbType string, celEnv *bcel.Env) ([]connectorbuilder.ResourceSyncer, error) {
	var ret []connectorbuilder.ResourceSyncer
	for rtID, rtConfig := range c.ResourceTypes {
		rt, err := c.GetResourceType(ctx, rtID)
		if err != nil {
			return nil, err
		}

		rv := &SQLSyncer{
			resourceType: rt,
			config:       rtConfig,
			db:           db,
			dbType:       dbType,
			env:          celEnv,
			fullConfig:   c,
		}
		ret = append(ret, rv)
	}

	return ret, nil
}
