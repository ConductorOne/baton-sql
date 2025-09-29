package connector

import (
	"context"
	"fmt"

	config_sdk "github.com/conductorone/baton-sdk/pb/c1/config/v1"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/actions"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sql/pkg/bsql"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
)

var ActionTypeMap = map[string]v2.ActionType{
	"unspecified":            v2.ActionType_ACTION_TYPE_UNSPECIFIED,
	"dynamic":                v2.ActionType_ACTION_TYPE_DYNAMIC,
	"account":                v2.ActionType_ACTION_TYPE_ACCOUNT,
	"account_update_profile": v2.ActionType_ACTION_TYPE_ACCOUNT_UPDATE_PROFILE,
	"account_disable":        v2.ActionType_ACTION_TYPE_ACCOUNT_DISABLE,
	"account_enable":         v2.ActionType_ACTION_TYPE_ACCOUNT_ENABLE,
}

func convertActionTypes(actionTypes []string) []v2.ActionType {
	var result []v2.ActionType
	for _, actionTypeStr := range actionTypes {
		if protoType, exists := ActionTypeMap[actionTypeStr]; exists {
			result = append(result, protoType)
		}
	}
	return result
}

// RegisterActionManager implements the RegisterActionManager interface to expose custom actions.
func (c *Connector) RegisterActionManager(ctx context.Context) (connectorbuilder.CustomActionManager, error) {
	l := ctxzap.Extract(ctx)

	if !c.config.HasActions() {
		return nil, nil
	}

	actionManager := actions.NewActionManager(ctx)

	for actionKey, actionCfg := range c.config.Actions {
		actionSchema := &v2.BatonActionSchema{
			Name:        actionKey,
			DisplayName: actionCfg.Name,
			Description: actionCfg.Description,
			Arguments:   []*config_sdk.Field{},
			ReturnTypes: []*config_sdk.Field{},
			ActionType:  convertActionTypes(actionCfg.ActionType),
		}

		for _, argCfg := range actionCfg.Arguments {
			arg := &config_sdk.Field{
				Name:        argCfg.Name,
				DisplayName: argCfg.DisplayName,
				Description: argCfg.Description,
				IsRequired:  argCfg.Required,
			}
			if arg.DisplayName == "" {
				arg.DisplayName = argCfg.Name
			}
			defaultValue := argCfg.Default
			switch argCfg.Type {
			case "string":
				stringField := &config_sdk.StringField{}
				if defaultValue != nil {
					stringField.DefaultValue = defaultValue.(string)
				}
				arg.Field = &config_sdk.Field_StringField{StringField: stringField}
			case "boolean":
				boolField := &config_sdk.BoolField{}
				if defaultValue != nil {
					boolField.DefaultValue = defaultValue.(bool)
				}
				arg.Field = &config_sdk.Field_BoolField{BoolField: boolField}
			case "number":
				intField := &config_sdk.IntField{}
				if defaultValue != nil {
					intField.DefaultValue = defaultValue.(int64)
				}
				arg.Field = &config_sdk.Field_IntField{IntField: intField}
			case "string_list":
				stringSliceField := &config_sdk.StringSliceField{}
				if defaultValue != nil {
					stringSliceField.DefaultValue = defaultValue.([]string)
				}
				arg.Field = &config_sdk.Field_StringSliceField{StringSliceField: stringSliceField}
			case "string_map":
				stringMapField := &config_sdk.StringMapField{}
				if defaultValue != nil {
					stringMapField.DefaultValue = defaultValue.(map[string]*anypb.Any)
				}
				arg.Field = &config_sdk.Field_StringMapField{StringMapField: stringMapField}
			}
			actionSchema.Arguments = append(actionSchema.Arguments, arg)
		}

		if actionCfg.Query == "" {
			return nil, fmt.Errorf("query is required for action: %s", actionKey)
		}

		err := actionManager.RegisterAction(ctx, actionKey, actionSchema, func(ctx context.Context, args *structpb.Struct) (*structpb.Struct, annotations.Annotations, error) {
			return c.handleQueryAction(ctx, actionCfg, actionSchema, args)
		})
		if err != nil {
			l.Error("failed to register action", zap.String("action", actionKey), zap.Error(err))
			return nil, err
		}
		l.Info("registered action", zap.String("action", actionKey), zap.Any("actionCfg", actionCfg))
	}

	return actionManager, nil
}

func (c *Connector) handleQueryAction(ctx context.Context, actionCfg bsql.ActionConfig, actionSchema *v2.BatonActionSchema, args *structpb.Struct) (*structpb.Struct, annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)
	l.Debug("actionHandler", zap.Any("actionCfg", actionCfg), zap.Any("actionSchema", actionSchema), zap.Any("args", args))

	sqlSyncer, err := bsql.NewActionSyncer(ctx, c.db, c.dbEngine, c.celEnv, *c.config)
	if err != nil {
		return nil, nil, err
	}

	var argMap = make(map[string]any)
	for k, v := range actionCfg.Arguments {
		switch v.Type {
		case "string":
			argMap[k] = args.Fields[v.Name].GetStringValue()
		case "boolean":
			argMap[k] = args.Fields[v.Name].GetBoolValue()
		case "number":
			argMap[k] = args.Fields[v.Name].GetNumberValue()
		case "string_list":
			argMap[k] = args.Fields[v.Name].GetListValue().Values
		case "string_map":
			argMap[k] = args.Fields[v.Name].GetStructValue().AsMap()
		default:
			return nil, nil, fmt.Errorf("unsupported argument type: %s", v.Type)
		}
	}

	queries := []string{actionCfg.Query}
	err = sqlSyncer.RunProvisioningQueries(ctx, queries, argMap, !actionCfg.NoTransaction)
	if err != nil {
		return nil, nil, err
	}

	return &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"success": {Kind: &structpb.Value_BoolValue{BoolValue: true}},
			"message": {Kind: &structpb.Value_StringValue{StringValue: "Action completed successfully"}},
		},
	}, nil, nil
}
