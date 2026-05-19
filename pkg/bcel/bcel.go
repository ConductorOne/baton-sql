package bcel

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/google/cel-go/cel"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	sdkResource "github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/conductorone/baton-sql/pkg/bcel/functions"
	"github.com/conductorone/baton-sql/pkg/helpers"
)

type Env struct {
	celEnv *cel.Env
}

func NewEnv(ctx context.Context) (*Env, error) {
	var celOpts []cel.EnvOption

	// resource and principal are MapType(string, any) so YAML can reach into profile sub-maps.
	celOpts = append(celOpts,
		cel.Variable("cols", cel.MapType(cel.StringType, cel.AnyType)),
		cel.Variable("resource", cel.MapType(cel.StringType, cel.AnyType)),
		cel.Variable("principal", cel.MapType(cel.StringType, cel.AnyType)),
		cel.Variable("entitlement", cel.MapType(cel.StringType, cel.AnyType)),
		cel.Variable("input", cel.MapType(cel.StringType, cel.AnyType)), // For action vars and account provisioning
	)

	// CEL functions
	celOpts = append(celOpts, functions.GetAllOptions()...)

	celEnv, err := cel.NewEnv(celOpts...)
	if err != nil {
		return nil, err
	}
	return &Env{
		celEnv: celEnv,
	}, nil
}

func (t *Env) Evaluate(ctx context.Context, expr string, inputs map[string]any) (any, error) {
	expr = preprocessExpressions(expr)

	ast, issues := t.celEnv.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return "", issues.Err()
	}

	prg, err := t.celEnv.Program(ast)
	if err != nil {
		return "", err
	}

	// Make sure that our input always has the 'cols' member
	if _, ok := inputs["cols"]; !ok {
		inputs["cols"] = make(map[string]any)
	}

	out, _, err := prg.ContextEval(ctx, inputs)
	if err != nil {
		return "", err
	}

	return out.Value(), nil
}

func (t *Env) EvaluateString(ctx context.Context, expr string, inputs map[string]any) (string, error) {
	out, err := t.Evaluate(ctx, expr, inputs)
	if err != nil {
		return "", err
	}

	switch ret := out.(type) {
	case string:
		return ret, nil
	case int64, int32, int, uint64, uint32, uint:
		return fmt.Sprintf("%d", ret), nil
	default:
		return fmt.Sprintf("%s", ret), nil
	}
}

func (t *Env) EvaluateBool(ctx context.Context, expr string, inputs map[string]any) (bool, error) {
	out, err := t.Evaluate(ctx, expr, inputs)
	if err != nil {
		return false, err
	}

	switch ret := out.(type) {
	case bool:
		return ret, nil
	case int64, int32, int, uint64, uint32, uint:
		return ret != 0, nil
	case string:
		parsed, err := strconv.ParseBool(ret)
		if err != nil {
			return false, fmt.Errorf("failed to parse bool from string %s: %w", ret, err)
		}
		return parsed, nil
	default:
		return false, fmt.Errorf("expected bool, got %T", out)
	}
}

func (t *Env) SyncInputs(rowMap map[string]any) map[string]any {
	ret := make(map[string]any)

	if rowMap != nil {
		ret["cols"] = rowMap
	}

	return ret
}

func (t *Env) SyncInputsWithResource(rowMap map[string]any, resource *v2.Resource) map[string]any {
	ret := t.SyncInputs(rowMap)

	if resource != nil {
		ret["resource"] = resourceToCELMap(resource)
	}

	return ret
}

func resourceToCELMap(resource *v2.Resource) map[string]any {
	out := map[string]any{
		"ID":          resource.Id.Resource,
		"Type":        resource.Id.ResourceType,
		"DisplayName": resource.DisplayName,
	}

	if t, err := sdkResource.GetGroupTrait(resource); err == nil && t.GetProfile() != nil {
		out["profile"] = t.GetProfile().AsMap()
	} else if t, err := sdkResource.GetUserTrait(resource); err == nil && t.GetProfile() != nil {
		out["profile"] = t.GetProfile().AsMap()
	} else if t, err := sdkResource.GetRoleTrait(resource); err == nil && t.GetProfile() != nil {
		out["profile"] = t.GetProfile().AsMap()
	} else if t, err := sdkResource.GetAppTrait(resource); err == nil && t.GetProfile() != nil {
		out["profile"] = t.GetProfile().AsMap()
	}

	// Empty default so `has(resource.profile.X)` is well-defined for optional fields.
	if _, exists := out["profile"]; !exists {
		out["profile"] = map[string]any{}
	}

	return out
}

func (t *Env) ProvisioningInputs(principal *v2.Resource, entitlement *v2.Entitlement) (map[string]any, error) {
	if principal == nil {
		return nil, errors.New("principal is required")
	}

	if entitlement == nil {
		return nil, errors.New("entitlement is required")
	}

	ret := make(map[string]any)

	ret["principal"] = resourceToCELMap(principal)

	resourceType, resourceID, entitlementID, err := helpers.SplitEntitlementID(entitlement)
	if err != nil {
		return nil, err
	}

	ret["entitlement"] = map[string]string{
		"ID": entitlementID,
	}

	// Fallback shape covers grants reconstructed from c1z without the full resource.
	if entitlement.Resource != nil {
		ret["resource"] = resourceToCELMap(entitlement.Resource)
	} else {
		ret["resource"] = map[string]any{
			"ID":      resourceID,
			"Type":    resourceType,
			"profile": map[string]any{},
		}
	}

	return ret, nil
}

func (t *Env) AccountProvisioningInputs(inputs map[string]any) (map[string]any, error) {
	ret := make(map[string]any)

	ret["input"] = inputs

	return ret, nil
}
