package connector

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

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
				switch v := defaultValue.(type) {
				case nil:
				case string:
					stringField.DefaultValue = v
				default:
					return nil, fmt.Errorf("invalid string default for %s: %T", actionKey, defaultValue)
				}
				arg.Field = &config_sdk.Field_StringField{StringField: stringField}
			case "boolean":
				boolField := &config_sdk.BoolField{}
				switch v := defaultValue.(type) {
				case nil:
				case bool:
					boolField.DefaultValue = v
				case string:
					defaultValue, err := strconv.ParseBool(v)
					if err != nil {
						return nil, fmt.Errorf("invalid boolean default for %s: %T", actionKey, defaultValue)
					}
					boolField.DefaultValue = defaultValue
				default:
					return nil, fmt.Errorf("invalid boolean default for %s: %T", actionKey, defaultValue)
				}
				arg.Field = &config_sdk.Field_BoolField{BoolField: boolField}
			case "number":
				intField := &config_sdk.IntField{}
				switch v := defaultValue.(type) {
				case nil:
				case int, int32, int64:
					intField.DefaultValue = reflect.ValueOf(v).Int()
				case float32, float64:
					intField.DefaultValue = int64(reflect.ValueOf(v).Float())
				default:
					return nil, fmt.Errorf("invalid numeric default for %s: %T", actionKey, defaultValue)
				}
				arg.Field = &config_sdk.Field_IntField{IntField: intField}
			case "string_list":
				stringSliceField := &config_sdk.StringSliceField{}
				switch v := defaultValue.(type) {
				case nil:
				case []string:
					stringSliceField.DefaultValue = v
				default:
					return nil, fmt.Errorf("invalid string slice default for %s: %T", actionKey, defaultValue)
				}
				arg.Field = &config_sdk.Field_StringSliceField{StringSliceField: stringSliceField}
			case "string_map":
				stringMapField := &config_sdk.StringMapField{}
				switch v := defaultValue.(type) {
				case nil:
				case map[string]*anypb.Any:
					stringMapField.DefaultValue = v
				default:
					return nil, fmt.Errorf("invalid string map default for %s: %T", actionKey, defaultValue)
				}
				arg.Field = &config_sdk.Field_StringMapField{StringMapField: stringMapField}
			}
			actionSchema.Arguments = append(actionSchema.Arguments, arg)
		}

		if actionCfg.Query == "" {
			return nil, fmt.Errorf("query is required for action: %s", actionKey)
		}

		cfg := actionCfg

		err := actionManager.RegisterAction(ctx, actionKey, actionSchema, func(ctx context.Context, args *structpb.Struct) (*structpb.Struct, annotations.Annotations, error) {
			return c.handleQueryAction(ctx, cfg, args)
		})
		if err != nil {
			l.Error("failed to register action", zap.String("action", actionKey), zap.Error(err))
			return nil, err
		}
		l.Info("registered action", zap.String("action", actionKey))
	}

	return actionManager, nil
}

func (c *Connector) handleQueryAction(ctx context.Context, actionCfg bsql.ActionConfig, args *structpb.Struct) (*structpb.Struct, annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)
	l.Debug("actionHandler", zap.String("action", actionCfg.Name))

	sqlSyncer, err := bsql.NewActionSyncer(ctx, c.db, c.dbEngine, c.celEnv, *c.config)
	if err != nil {
		return nil, nil, err
	}

	var argMap = make(map[string]any)
	for k, v := range actionCfg.Arguments {
		if _, ok := args.Fields[v.Name]; !ok {
			if v.Required {
				return nil, nil, fmt.Errorf("argument %s is required", v.Name)
			}
			if v.Default != nil {
				argMap[k] = v.Default
			}
			continue
		}
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
