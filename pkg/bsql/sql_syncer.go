package bsql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	sdkResource "github.com/conductorone/baton-sdk/pkg/types/resource"
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
}

func (s *SQLSyncer) mapResource(ctx context.Context, rowMap map[string]interface{}) (*v2.Resource, error) {
	traits := make(map[string]bool)
	mapTraits := s.config.List.Map.Traits
	if mapTraits != nil {
		if mapTraits.User != nil {
			traits["user"] = true
		} else if mapTraits.Group != nil {
			traits["group"] = true
		} else if mapTraits.Role != nil {
			traits["role"] = true
		} else if mapTraits.App != nil {
			traits["app"] = true
		}
	}

	annos := annotations.Annotations{}

	ret := &v2.Resource{}
	rID, err := s.getMappedID(ctx, rowMap)
	if err != nil {
		return nil, err
	}
	ret.Id, err = sdkResource.NewResourceID(s.resourceType, rID)
	if err != nil {
		return nil, err
	}

	displayName, err := s.getMappedDisplayName(ctx, rowMap)
	if err != nil {
		return nil, err
	}
	ret.DisplayName = displayName

	if traits["user"] {
		ut, err := sdkResource.NewUserTrait()
		if err != nil {
			return nil, err
		}
		annos.Update(ut)
	}

	if traits["role"] {
		rt, err := sdkResource.NewRoleTrait()
		if err != nil {
			return nil, err
		}
		annos.Update(rt)
	}

	if traits["group"] {
		gt, err := sdkResource.NewGroupTrait()
		if err != nil {
			return nil, err
		}
		annos.Update(gt)
	}

	if traits["app"] {
		at, err := sdkResource.NewAppTrait()
		if err != nil {
			return nil, err
		}
		annos.Update(at)
	}

	ret.Annotations = annos
	return ret, nil
}

func (s *SQLSyncer) getMappedID(ctx context.Context, rowMap map[string]interface{}) (string, error) {
	mapping := s.config.List.Map.Id
	if mapping == "" {
		return "", errors.New("no ID mapping provided")
	}

	if strings.HasPrefix(mapping, ".") {
		if v, ok := rowMap[mapping[1:]]; ok {
			return fmt.Sprintf("%s", v), nil
		} else {
			return "", errors.New("ID mapping not found in row")
		}
	} else {
		return mapping, nil
	}
}

func (s *SQLSyncer) getMappedDisplayName(ctx context.Context, rowMap map[string]interface{}) (string, error) {
	mapping := s.config.List.Map.DisplayName
	if mapping == "" {
		return "", errors.New("no display name mapping provided")
	}

	if strings.HasPrefix(mapping, ".") {
		if v, ok := rowMap[mapping[1:]]; ok {
			return fmt.Sprintf("%s", v), nil
		} else {
			return "", errors.New("display name mapping not found in row")
		}
	} else {
		return mapping, nil
	}
}

func (s *SQLSyncer) ResourceType(ctx context.Context) *v2.ResourceType {
	return s.resourceType
}

func (s *SQLSyncer) List(ctx context.Context, parentResourceID *v2.ResourceId, pToken *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	limit := pToken.Size
	if limit == 0 {
		limit = 100
	}

	qCtx := map[string]string{
		"limit": strconv.Itoa(limit),
	}

	if pToken.Token != "" {
		qCtx["offset"] = pToken.Token
	} else {
		qCtx["offset"] = "0"
	}

	var ret []*v2.Resource

	q, err := parseQueryOpts(ctx, s.config.List.Query, qCtx)
	if err != nil {
		return nil, "", nil, err
	}

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, "", nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, "", nil, err
	}

	values := make([]interface{}, len(columns))
	scanArgs := make([]interface{}, len(values))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	for rows.Next() {
		if err := rows.Scan(scanArgs...); err != nil {
			return nil, "", nil, err
		}

		rowMap := make(map[string]interface{})
		for i, colName := range columns {
			rowMap[colName] = values[i]
		}

		r, err := s.mapResource(ctx, rowMap)
		if err != nil {
			return nil, "", nil, err
		}
		ret = append(ret, r)
	}

	if err := rows.Err(); err != nil {
		return nil, "", nil, err
	}

	return ret, "", nil, nil
}

func (s *SQLSyncer) Entitlements(ctx context.Context, resource *v2.Resource, pToken *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

func (s *SQLSyncer) Grants(ctx context.Context, resource *v2.Resource, pToken *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

func (c Config) GetSQLSyncers(ctx context.Context, db *sql.DB) ([]connectorbuilder.ResourceSyncer, error) {
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
		}
		ret = append(ret, rv)
	}

	return ret, nil
}
