package bsql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

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

var queryOptRegex = regexp.MustCompile(`\?\<([a-zA-Z0-9_]+)\>`)

func parseQueryOpts(ctx context.Context, query string, values map[string]string) (string, error) {
	var parseErr error
	updatedQuery := queryOptRegex.ReplaceAllStringFunc(query, func(token string) string {
		key := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(token, "?<"), ">"))

		if v, ok := values[key]; ok {
			return v
		}

		parseErr = errors.Join(parseErr, fmt.Errorf("missing value for token %s", token))
		return token
	})
	if parseErr != nil {
		return "", parseErr
	}
	return updatedQuery, nil
}

type SQLSyncer struct {
	resourceType *v2.ResourceType
	db           *sql.DB
	config       ResourceType
	env          *bcel.Env
	fullConfig   Config
}

func (s *SQLSyncer) ResourceType(ctx context.Context) *v2.ResourceType {
	return s.resourceType
}

func (c Config) GetSQLSyncers(ctx context.Context, db *sql.DB, celEnv *bcel.Env) ([]connectorbuilder.ResourceSyncer, error) {
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
			env:          celEnv,
			fullConfig:   c,
		}
		ret = append(ret, rv)
	}

	return ret, nil
}
