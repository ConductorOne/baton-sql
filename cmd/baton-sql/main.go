package main

import (
	"context"
	"fmt"
	"os"

	configSdk "github.com/conductorone/baton-sdk/pkg/config"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/field"
	"github.com/conductorone/baton-sdk/pkg/types"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/conductorone/baton-sql/pkg/config"
	"github.com/conductorone/baton-sql/pkg/connector"
)

var version = "dev"

func main() {
	ctx := context.Background()

	_, cmd, err := configSdk.DefineConfiguration(
		ctx,
		"baton-sql",
		getConnector,
		field.Configuration{
			Fields: config.ConfigurationFields,
		},
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	cmd.Version = version

	err = cmd.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func getConnector(ctx context.Context, v *viper.Viper) (types.ConnectorServer, error) {
	l := ctxzap.Extract(ctx)

	var opts []connector.NewOption

	// Apply app-name override if provided
	if appName := v.GetString("app-name"); appName != "" {
		opts = append(opts, connector.WithAppName(appName))
	}

	// Apply app-description override if provided
	if appDescription := v.GetString("app-description"); appDescription != "" {
		opts = append(opts, connector.WithAppDescription(appDescription))
	}

	cb, err := connector.New(ctx, v.GetString("config-path"), opts...)
	if err != nil {
		l.Error("error creating connector", zap.Error(err))
		return nil, err
	}
	connector, err := connectorbuilder.NewConnector(ctx, cb)
	if err != nil {
		l.Error("error creating connector", zap.Error(err))
		return nil, err
	}
	return connector, nil
}
