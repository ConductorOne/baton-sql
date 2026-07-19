package connector

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	dbs      map[string]*sql.DB
	dbEngine database.DbEngine
	celEnv   *bcel.Env
}

func (c *Connector) Close() error {
	var errs error
	for _, db := range c.dbs {
		if db == nil {
			continue
		}
		if err := db.Close(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

// ResourceSyncers returns a ResourceSyncer for each resource type that should be synced from the upstream service.
func (c *Connector) ResourceSyncers(ctx context.Context) []connectorbuilder.ResourceSyncer {
	syncers, err := c.config.GetSQLSyncers(ctx, c.dbs, c.dbEngine, c.celEnv)
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
	syncers, err := c.config.GetSQLSyncers(ctx, c.dbs, c.dbEngine, c.celEnv)
	if err != nil {
		return nil, err
	}

	for _, syncer := range syncers {
		if v, ok := syncer.(interface {
			Validate(ctx context.Context) error
		}); ok {
			err := v.Validate(ctx)
			if err != nil {
				return nil, err
			}
		}
	}

	for name, db := range c.dbs {
		if err := db.PingContext(ctx); err != nil {
			return nil, fmt.Errorf("database %q ping failed: %w", name, err)
		}
	}
	return nil, nil
}

// New returns a new instance of the connector.
func New(ctx context.Context, configFilePath string, opts ...NewOption) (*Connector, error) {
	c, err := bsql.LoadConfigFromFile(configFilePath)
	if err != nil {
		return nil, err
	}

	// Apply options to override config values
	for _, opt := range opts {
		opt(c)
	}

	return newConnector(ctx, c)
}

// NewOption is a function that modifies the config.
type NewOption func(*bsql.Config)

// WithAppName sets the app name, overriding the config file value.
func WithAppName(name string) NewOption {
	return func(c *bsql.Config) {
		if name != "" {
			c.AppName = name
		}
	}
}

// WithAppDescription sets the app description, overriding the config file value.
func WithAppDescription(description string) NewOption {
	return func(c *bsql.Config) {
		if description != "" {
			c.AppDescription = description
		}
	}
}

func newConnector(ctx context.Context, c *bsql.Config) (*Connector, error) {
	opts := database.ConnectOptions{
		DSN:      c.Connect.DSN,
		Scheme:   c.Connect.Scheme,
		Host:     c.Connect.Host,
		Port:     c.Connect.Port,
		Database: c.Connect.Database,
		User:     c.Connect.User,
		Password: c.Connect.Password,
		Params:   c.Connect.Params,
	}

	dbs, dbEngine, err := openDatabases(ctx, opts, c.Connect.Databases)
	if err != nil {
		return nil, err
	}

	celEnv, err := bcel.NewEnv(ctx)
	if err != nil {
		for _, db := range dbs {
			_ = db.Close()
		}
		return nil, err
	}

	return &Connector{
		config:   c,
		dbs:      dbs,
		dbEngine: dbEngine,
		celEnv:   celEnv,
	}, nil
}

// openDatabases returns one *sql.DB per database to sync. Consumers derive their own
// sorted view via sortedDBNames; this function does not pre-sort.
func openDatabases(
	ctx context.Context,
	opts database.ConnectOptions,
	dbsCfg *bsql.DatabasesConfig,
) (map[string]*sql.DB, database.DbEngine, error) {
	if dbsCfg == nil {
		db, dbEngine, err := database.Connect(ctx, opts)
		if err != nil {
			return nil, database.Unknown, err
		}
		key := database.ResolveDatabaseName(opts)
		return map[string]*sql.DB{key: db}, dbEngine, nil
	}

	if err := dbsCfg.Validate(); err != nil {
		return nil, database.Unknown, err
	}

	dbNames := dbsCfg.Static
	if dbsCfg.DiscoveryQuery != "" {
		adminDB, _, err := database.Connect(ctx, opts)
		if err != nil {
			return nil, database.Unknown, fmt.Errorf("databases.discovery_query: admin connect failed: %w", err)
		}
		discovered, discoverErr := runDiscoveryQuery(ctx, adminDB, dbsCfg.DiscoveryQuery)
		if cerr := adminDB.Close(); cerr != nil && discoverErr == nil {
			discoverErr = fmt.Errorf("admin handle close: %w", cerr)
		}
		if discoverErr != nil {
			return nil, database.Unknown, discoverErr
		}
		if len(discovered) == 0 {
			return nil, database.Unknown, errors.New("databases.discovery_query: returned zero rows")
		}
		dbNames = discovered
	}

	return database.ConnectMany(ctx, opts, dbNames)
}

func runDiscoveryQuery(ctx context.Context, db *sql.DB, query string) ([]string, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("databases.discovery_query: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("databases.discovery_query: scan: %w", err)
		}
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("databases.discovery_query: rows: %w", err)
	}
	return names, nil
}
