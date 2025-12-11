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

	actionManager := actions.NewActionManager(ctx)

	if !c.config.HasActions() {
		return actionManager, nil
	}

	for actionKey, actionCfg := range c.config.Actions {
		actionSchema := &v2.BatonActionSchema{
			Name:        actionKey,
			DisplayName: actionCfg.Name,
			Description: actionCfg.Description,
			Arguments:   []*config_sdk.Field{},
			ReturnTypes: []*config_sdk.Field{},
			ActionType:  convertActionTypes(actionCfg.ActionType),
		}

		for k, argCfg := range actionCfg.Arguments {
			arg := &config_sdk.Field{
				Name:        k,
				DisplayName: argCfg.Name,
				Description: argCfg.Description,
				IsRequired:  argCfg.Required,
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

		cfg := actionCfg

		// Validate the action config
		if err := cfg.Validate(); err != nil {
			return nil, fmt.Errorf("invalid action config %s: %w", actionKey, err)
		}

		err := actionManager.RegisterAction(ctx, actionKey, actionSchema, func(ctx context.Context, args *structpb.Struct) (*structpb.Struct, annotations.Annotations, error) {
			return c.handleQueryAction(ctx, actionKey, cfg, args)
		})
		if err != nil {
			l.Error("failed to register action", zap.String("action", actionKey), zap.Error(err))
			return nil, err
		}
		l.Info("registered action", zap.String("action", actionKey))
	}

	return actionManager, nil
}

func (c *Connector) handleQueryAction(ctx context.Context, actionKey string, actionCfg bsql.ActionConfig, args *structpb.Struct) (*structpb.Struct, annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)
	l.Debug("actionHandler", zap.String("action", actionKey))

	sqlSyncer, err := bsql.NewActionSyncer(ctx, c.db, c.dbEngine, c.celEnv, *c.config)
	if err != nil {
		return nil, nil, err
	}

	var argMap = make(map[string]any)
	for k, v := range actionCfg.Arguments {
		if _, ok := args.Fields[k]; !ok {
			if v.Required {
				return nil, nil, fmt.Errorf("argument %s is required", k)
			}
			if v.Default != nil {
				argMap[k] = v.Default
			}
			continue
		}
		switch v.Type {
		case "string":
			argMap[k] = args.Fields[k].GetStringValue()
		case "boolean":
			argMap[k] = args.Fields[k].GetBoolValue()
		case "number":
			argMap[k] = args.Fields[k].GetNumberValue()
		case "string_list":
			values := args.Fields[k].GetListValue().GetValues()
			var stringList []string
			for _, value := range values {
				stringList = append(stringList, value.GetStringValue())
			}
			argMap[k] = stringList
		case "string_map":
			argMap[k] = args.Fields[k].GetStructValue().AsMap()
		default:
			return nil, nil, fmt.Errorf("argument %s has unsupported type: %s", k, v.Type)
		}
	}

	// Wrap argMap in "input" container for CEL expressions
	celInputs := map[string]any{
		"input": argMap,
	}

	// Evaluate CEL expressions in vars to prepare query variables
	queryVars, err := sqlSyncer.PrepareQueryVars(ctx, celInputs, actionCfg.Vars)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to prepare query vars: %w", err)
	}

	// Merge evaluated vars into argMap (queryVars take precedence)
	for k, v := range queryVars {
		argMap[k] = v
	}

	var queries []string
	if len(actionCfg.Queries) > 0 {
		queries = actionCfg.Queries
	} else {
		queries = []string{actionCfg.Query}
	}
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
