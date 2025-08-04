package config

import "github.com/conductorone/baton-sdk/pkg/field"

var (
	ConfigPathField = field.StringField(
		"config-path",
		field.WithRequired(true),
		field.WithDescription("The file path to the baton-sql config to use"),
	)

	// ConfigurationFields defines the external configuration required for the connector to run.
	ConfigurationFields = []field.SchemaField{
		ConfigPathField,
	}
	ConfigurationSchema = field.NewConfiguration(ConfigurationFields)
)

var (
	Config = field.NewConfiguration(
		ConfigurationFields,
		field.WithConnectorDisplayName("SQL"),
		field.WithHelpUrl("/docs/baton/sql"),
		field.WithIconUrl("/static/app-icons/sql.svg"),
	)
)
