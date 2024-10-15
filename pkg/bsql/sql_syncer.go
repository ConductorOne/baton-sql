package bsql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	sdkResource "github.com/conductorone/baton-sdk/pkg/types/resource"

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
}

func (s *SQLSyncer) fetchTraits(ctx context.Context) map[string]bool {
	traits := make(map[string]bool)
	mapTraits := s.config.List.Map.Traits
	if mapTraits != nil {
		switch {
		case mapTraits.User != nil:
			traits[userTraitType] = true

		case mapTraits.Group != nil:
			traits[groupTraitType] = true

		case mapTraits.Role != nil:
			traits[roleTraitType] = true

		case mapTraits.App != nil:
			traits[appTraitType] = true
		}
	}

	return traits
}

func (s *SQLSyncer) mapUserTrait(ctx context.Context, r *v2.Resource, rowMap map[string]any) error {
	l := ctxzap.Extract(ctx)

	inputs, err := s.env.BaseInputs(rowMap)
	if err != nil {
		return err
	}

	mappings := s.config.List.Map.Traits.User

	var opts []sdkResource.UserTraitOption

	// Emails
	for ii, mapping := range mappings.Emails {
		if mapping == "" {
			l.Warn("missing email mapping configuration for user trait", zap.Int("index", ii))
			continue
		}

		// Make the first email listed in the mapping the primary
		primary := false
		if ii == 0 {
			primary = true
		}

		v, err := s.env.EvaluateString(ctx, mapping, inputs)
		if err != nil {
			return err
		}

		opts = append(opts, sdkResource.WithEmail(v, primary))
	}

	// Status
	if mappings.Status != "" {
		statusValue, err := s.env.EvaluateString(ctx, mappings.Status, inputs)
		if err != nil {
			return err
		}

		var status v2.UserTrait_Status_Status
		switch strings.ToLower(statusValue) {
		case "active":
			status = v2.UserTrait_Status_STATUS_ENABLED
		case "enabled":
			status = v2.UserTrait_Status_STATUS_ENABLED
		case "disabled":
			status = v2.UserTrait_Status_STATUS_DISABLED
		case "inactive":
			status = v2.UserTrait_Status_STATUS_DISABLED
		case "suspended":
			status = v2.UserTrait_Status_STATUS_DISABLED
		case "locked":
			status = v2.UserTrait_Status_STATUS_DISABLED
		case "deleted":
			status = v2.UserTrait_Status_STATUS_DELETED
		default:
			l.Warn("unexpected status value in mapping", zap.String("status", statusValue))
			status = v2.UserTrait_Status_STATUS_UNSPECIFIED
		}

		if mappings.StatusDetails != "" {
			v, err := s.env.EvaluateString(ctx, mappings.StatusDetails, inputs)
			if err != nil {
				return err
			}
			opts = append(opts, sdkResource.WithDetailedStatus(status, v))
		} else {
			opts = append(opts, sdkResource.WithStatus(status))
		}
	}

	profile := make(map[string]interface{})
	for profileKey, profileValue := range mappings.Profile {
		v, err := s.env.EvaluateString(ctx, profileValue, inputs)
		if err != nil {
			return err
		}
		profile[profileKey] = v
	}

	if len(profile) > 0 {
		opts = append(opts, sdkResource.WithUserProfile(profile))
	}

	if mappings.AccountType != "" {
		v, err := s.env.EvaluateString(ctx, mappings.AccountType, inputs)
		if err != nil {
			return err
		}

		var accountType v2.UserTrait_AccountType
		switch strings.ToLower(v) {
		case "user":
			accountType = v2.UserTrait_ACCOUNT_TYPE_HUMAN
		case "human":
			accountType = v2.UserTrait_ACCOUNT_TYPE_HUMAN
		case "service":
			accountType = v2.UserTrait_ACCOUNT_TYPE_SERVICE
		case "system":
			accountType = v2.UserTrait_ACCOUNT_TYPE_SYSTEM
		default:
			l.Warn("unexpected account type value in mapping, defaulting to human", zap.String("account_type", v))
			accountType = v2.UserTrait_ACCOUNT_TYPE_HUMAN
		}
		opts = append(opts, sdkResource.WithAccountType(accountType))
	} else {
		// If no mapping is provided, default to human
		opts = append(opts, sdkResource.WithAccountType(v2.UserTrait_ACCOUNT_TYPE_HUMAN))
	}

	if mappings.Login != "" {
		primaryLogin, err := s.env.EvaluateString(ctx, mappings.Login, inputs)
		if err != nil {
			return err
		}

		aliases := make([]string, 0)
		for _, a := range mappings.LoginAliases {
			alias, err := s.env.EvaluateString(ctx, a, inputs)
			if err != nil {
				return err
			}
			if alias != "" {
				aliases = append(aliases, alias)
			}
		}
		opts = append(opts, sdkResource.WithUserLogin(primaryLogin, aliases...))
	}

	t, err := sdkResource.NewUserTrait(opts...)
	if err != nil {
		return err
	}

	annos := annotations.Annotations(r.Annotations)
	annos.Update(t)
	r.Annotations = annos

	return nil
}

func (s *SQLSyncer) mapAppTrait(ctx context.Context, r *v2.Resource, rowMap map[string]any) error {
	inputs, err := s.env.BaseInputs(rowMap)
	if err != nil {
		return err
	}

	mappings := s.config.List.Map.Traits.App

	var opts []sdkResource.AppTraitOption

	if mappings.HelpUrl != "" {
		v, err := s.env.EvaluateString(ctx, mappings.HelpUrl, inputs)
		if err != nil {
			return err
		}
		opts = append(opts, sdkResource.WithAppHelpURL(v))
	}

	profile := make(map[string]interface{})
	for profileKey, profileValue := range mappings.Profile {
		v, err := s.env.EvaluateString(ctx, profileValue, inputs)
		if err != nil {
			return err
		}
		profile[profileKey] = v
	}

	if len(profile) > 0 {
		opts = append(opts, sdkResource.WithAppProfile(profile))
	}

	t, err := sdkResource.NewAppTrait(opts...)
	if err != nil {
		return err
	}

	annos := annotations.Annotations(r.Annotations)
	annos.Update(t)
	r.Annotations = annos

	return nil
}

func (s *SQLSyncer) mapGroupTrait(ctx context.Context, r *v2.Resource, rowMap map[string]any) error {
	inputs, err := s.env.BaseInputs(rowMap)
	if err != nil {
		return err
	}

	mappings := s.config.List.Map.Traits.Group

	var opts []sdkResource.GroupTraitOption

	profile := make(map[string]interface{})
	for profileKey, profileValue := range mappings.Profile {
		v, err := s.env.EvaluateString(ctx, profileValue, inputs)
		if err != nil {
			return err
		}
		profile[profileKey] = v
	}
	if len(profile) > 0 {
		opts = append(opts, sdkResource.WithGroupProfile(profile))
	}

	t, err := sdkResource.NewGroupTrait(opts...)
	if err != nil {
		return err
	}

	annos := annotations.Annotations(r.Annotations)
	annos.Update(t)
	r.Annotations = annos

	return nil
}

func (s *SQLSyncer) mapRoleTrait(ctx context.Context, r *v2.Resource, rowMap map[string]any) error {
	inputs, err := s.env.BaseInputs(rowMap)
	if err != nil {
		return err
	}

	mappings := s.config.List.Map.Traits.Role

	var opts []sdkResource.RoleTraitOption

	profile := make(map[string]interface{})
	for profileKey, profileValue := range mappings.Profile {
		v, err := s.env.EvaluateString(ctx, profileValue, inputs)
		if err != nil {
			return err
		}
		profile[profileKey] = v
	}
	if len(profile) > 0 {
		opts = append(opts, sdkResource.WithRoleProfile(profile))
	}

	t, err := sdkResource.NewRoleTrait(opts...)
	if err != nil {
		return err
	}

	annos := annotations.Annotations(r.Annotations)
	annos.Update(t)
	r.Annotations = annos

	return nil
}

func (s *SQLSyncer) mapTraits(ctx context.Context, r *v2.Resource, rowMap map[string]any) error {
	l := ctxzap.Extract(ctx)

	for trait, enabled := range s.fetchTraits(ctx) {
		if !enabled {
			continue
		}

		switch trait {
		case userTraitType:
			if err := s.mapUserTrait(ctx, r, rowMap); err != nil {
				return err
			}
		case roleTraitType:
			if err := s.mapRoleTrait(ctx, r, rowMap); err != nil {
				return err
			}
		case appTraitType:
			if err := s.mapAppTrait(ctx, r, rowMap); err != nil {
				return err
			}
		case groupTraitType:
			if err := s.mapGroupTrait(ctx, r, rowMap); err != nil {
				return err
			}
		default:
			l.Warn("unexpected trait type in mapping", zap.String("trait", trait))
			continue
		}
	}

	return nil
}

func (s *SQLSyncer) mapResource(ctx context.Context, rowMap map[string]any) (*v2.Resource, error) {
	r := &v2.Resource{}

	err := s.getMappedResource(ctx, r, rowMap)
	if err != nil {
		return nil, err
	}

	err = s.mapTraits(ctx, r, rowMap)
	if err != nil {
		return nil, err
	}

	return r, nil
}

func (s *SQLSyncer) getMappedResource(ctx context.Context, r *v2.Resource, rowMap map[string]interface{}) error {
	mapping := s.config.List.Map
	if mapping == nil {
		return errors.New("no mapping configuration provided")
	}

	inputs, err := s.env.BaseInputs(rowMap)
	if err != nil {
		return err
	}
	// Map ID
	if mapping.Id == "" {
		return errors.New("no ID mapping configuration provided")
	}
	v, err := s.env.EvaluateString(ctx, mapping.Id, inputs)
	if err != nil {
		return err
	}

	r.Id, err = sdkResource.NewResourceID(s.resourceType, v)
	if err != nil {
		return err
	}

	// Map Displayname
	if mapping.DisplayName == "" {
		return errors.New("no display name mapping configuration provided")
	}
	v, err = s.env.EvaluateString(ctx, mapping.DisplayName, inputs)
	if err != nil {
		return err
	}
	r.DisplayName = v

	// Map Description
	if mapping.Description != "" {
		v, err = s.env.EvaluateString(ctx, mapping.Description, inputs)
		if err != nil {
			return err
		}
		r.Description = v
	}

	return nil
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
		}
		ret = append(ret, rv)
	}

	return ret, nil
}
