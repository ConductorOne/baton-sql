package bsql

import (
	"context"
	"errors"
	"strings"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	sdkResource "github.com/conductorone/baton-sdk/pkg/types/resource"
)

func (s *SQLSyncer) List(ctx context.Context, parentResourceID *v2.ResourceId, pToken *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	var ret []*v2.Resource

	if s.config.List == nil {
		return nil, "", nil, errors.New("no resource list configuration provided")
	}

	queryVars, err := s.prepareQueryVars(ctx, nil, s.config.List.Vars)
	if err != nil {
		return nil, "", nil, err
	}

	npt, err := s.runQuery(ctx, pToken, s.config.List.Query, s.config.List.Pagination, queryVars, func(ctx context.Context, rowMap map[string]any) (bool, error) {
		r, err := s.mapResource(ctx, rowMap)
		if err != nil {
			return false, err
		}
		ret = append(ret, r)
		return true, nil
	})
	if err != nil {
		return nil, "", nil, err
	}

	return ret, npt, nil, nil
}

func (s *SQLSyncer) fetchTraits() map[string]bool {
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

	inputs := s.env.SyncInputs(rowMap)

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

	// Last Login
	if mappings.LastLogin != "" {
		lastLoginValue, err := s.env.EvaluateString(ctx, mappings.LastLogin, inputs)
		if err != nil {
			return err
		}

		if lastLoginValue != "" {
			// Try to parse the last login date using dbEngine to determine format
			lastLoginTime, err := parseTimeWithEngine(lastLoginValue, s.dbEngine)
			if err != nil {
				l.Warn("failed to parse last login time", zap.String("last_login", lastLoginValue), zap.Error(err))
			} else {
				opts = append(opts, sdkResource.WithLastLogin(*lastLoginTime))
			}
		}
	}

	// Employee ID
	if len(mappings.EmployeeIDs) > 0 {
		var employeeIDs []string
		for _, idMapping := range mappings.EmployeeIDs {
			employeeID, err := s.env.EvaluateString(ctx, idMapping, inputs)
			if err != nil {
				return err
			}
			if employeeID != "" {
				employeeIDs = append(employeeIDs, employeeID)
			}
		}

		if len(employeeIDs) > 0 {
			opts = append(opts, sdkResource.WithEmployeeID(employeeIDs...))
		}
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

	// Manager ID
	if mappings.ManagerID != "" {
		managerID, err := s.env.EvaluateString(ctx, mappings.ManagerID, inputs)
		if err != nil {
			return err
		}

		if managerID != "" {
			// Add manager ID to profile attributes
			profile["manager_id"] = managerID
		}
	}

	// Manager Email
	if mappings.ManagerEmail != "" {
		managerEmail, err := s.env.EvaluateString(ctx, mappings.ManagerEmail, inputs)
		if err != nil {
			return err
		}

		if managerEmail != "" {
			// Add manager email to profile attributes
			profile["manager_email"] = managerEmail
		}
	}

	t, err := sdkResource.NewUserTrait(opts...)
	if err != nil {
		return err
	}

	// Trait created successfully

	annos := annotations.Annotations(r.Annotations)
	annos.Update(t)
	r.Annotations = annos

	// Annotation applied

	return nil
}

func (s *SQLSyncer) mapAppTrait(ctx context.Context, r *v2.Resource, rowMap map[string]any) error {
	inputs := s.env.SyncInputs(rowMap)

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
	inputs := s.env.SyncInputs(rowMap)

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
	inputs := s.env.SyncInputs(rowMap)

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

	for trait, enabled := range s.fetchTraits() {
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

	inputs := s.env.SyncInputs(rowMap)

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
