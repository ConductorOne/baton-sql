package connector

import (
	"context"
	"database/sql"
	"errors"
	"io"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"

	"github.com/conductorone/baton-sql/pkg/bcel"
	"github.com/conductorone/baton-sql/pkg/bsql"
	"github.com/conductorone/baton-sql/pkg/database"
)

type Connector struct {
	config   *bsql.Config
	db       *sql.DB
	dbEngine database.DbEngine
	celEnv   *bcel.Env
}

func (c *Connector) Close() error {
	var errs error
	if c.db != nil {
		err := c.db.Close()
		if err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

// ResourceSyncers returns a ResourceSyncer for each resource type that should be synced from the upstream service.
func (c *Connector) ResourceSyncers(ctx context.Context) []connectorbuilder.ResourceSyncer {
	syncers, err := c.config.GetSQLSyncers(ctx, c.db, c.dbEngine, c.celEnv)
	if err != nil {
		return nil
	}

	return syncers
}

// Asset takes an input AssetRef and attempts to fetch it using the connector's authenticated http client
// It streams a response, always starting with a metadata object, following by chunked payloads for the asset.
func (c *Connector) Asset(ctx context.Context, asset *v2.AssetRef) (string, io.ReadCloser, error) {
	return "", nil, nil
}

// Metadata returns metadata about the connector.
func (c *Connector) Metadata(ctx context.Context) (*v2.ConnectorMetadata, error) {
	md := &v2.ConnectorMetadata{
		DisplayName: "Generic SQL Connector",
		Description: "A baton connector that allows you to sync from an arbitrary SQL database",
	}

	if c.config.AppName != "" {
		md.DisplayName = c.config.AppName
	}

	if c.config.AppDescription != "" {
		md.Description = c.config.AppDescription
	}

	accountCreationSchema, err := c.config.GetAccountCreationSchema(ctx)
	if err != nil {
		return nil, err
	}

	md.AccountCreationSchema = accountCreationSchema
	return md, nil
}

// Validate is called to ensure that the connector is properly configured. It should exercise any API credentials
// to be sure that they are valid.
func (c *Connector) Validate(ctx context.Context) (annotations.Annotations, error) {
	return nil, nil
}

// New returns a new instance of the connector.
func New(ctx context.Context, configFilePath string) (*Connector, error) {
	c, err := bsql.LoadConfigFromFile(configFilePath)
	if err != nil {
		return nil, err
	}

	return newConnector(ctx, c)
}

func newConnector(ctx context.Context, c *bsql.Config) (*Connector, error) {
	db, dbEngine, err := database.Connect(ctx, c.Connect.DSN, c.Connect.User, c.Connect.Password)
	if err != nil {
		return nil, err
	}

	celEnv, err := bcel.NewEnv(ctx)
	if err != nil {
		return nil, err
	}

	return &Connector{
		config:   c,
		db:       db,
		dbEngine: dbEngine,
		celEnv:   celEnv,
	}, nil
}
