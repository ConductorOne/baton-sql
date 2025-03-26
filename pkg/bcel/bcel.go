package bcel

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sql/pkg/bcel/functions"
	"github.com/conductorone/baton-sql/pkg/helpers"
)

type Env struct {
	celEnv *cel.Env
}

func NewEnv(ctx context.Context) (*Env, error) {
	var celOpts []cel.EnvOption

	// CEL variables
	celOpts = append(celOpts,
		cel.Variable("cols", cel.MapType(cel.StringType, cel.AnyType)),
		cel.Variable("resource", cel.MapType(types.StringType, types.StringType)),
		cel.Variable("principal", cel.MapType(types.StringType, types.StringType)),
		cel.Variable("entitlement", cel.MapType(types.StringType, types.StringType)),
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
		ret["resource"] = map[string]string{
			"ID":          resource.Id.Resource,
			"Type":        resource.Id.ResourceType,
			"DisplayName": resource.DisplayName,
		}
	}

	return ret
}

func (t *Env) ProvisioningInputs(principal *v2.Resource, entitlement *v2.Entitlement) (map[string]any, error) {
	if principal == nil {
		return nil, errors.New("principal is required")
	}

	if entitlement == nil {
		return nil, errors.New("entitlement is required")
	}

	ret := make(map[string]any)

	ret["principal"] = map[string]string{
		"ID":   principal.Id.Resource,
		"Type": principal.Id.ResourceType,
	}

	resourceType, resourceID, entitlementID, err := helpers.SplitEntitlementID(entitlement)
	if err != nil {
		return nil, err
	}

	ret["entitlement"] = map[string]string{
		"ID": entitlementID,
	}

	ret["resource"] = map[string]string{
		"ID":   resourceID,
		"Type": resourceType,
	}

	return ret, nil
}

func (t *Env) AccountProvisioningInputs(inputs map[string]any) (map[string]any, error) {
	ret := make(map[string]any)

	ret["input"] = inputs

	return ret, nil
}
