package config

import "github.com/conductorone/baton-sdk/pkg/field"

var (
	ConfigPathField = field.StringField(
		"config-path",
		field.WithRequired(true),
		field.WithDescription("The file path to the baton-sql config to use"),
	)

	AppNameField = field.StringField(
		"app-name",
		field.WithRequired(false),
		field.WithDescription("Override the app_name from the config file"),
	)

	AppDescriptionField = field.StringField(
		"app-description",
		field.WithRequired(false),
		field.WithDescription("Override the app_description from the config file"),
	)

	// ConfigurationFields defines the external configuration required for the connector to run.
	ConfigurationFields = []field.SchemaField{
		ConfigPathField,
		AppNameField,
		AppDescriptionField,
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
