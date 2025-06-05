package bsql

import (
	"context"
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
)

// Config represents the overall connector configuration.
type Config struct {
	// AppName is the application name that identifies the connector.
	AppName string `yaml:"app_name" json:"app_name"`

	// AppDescription provides an optional description of the application.
	AppDescription string `yaml:"app_description" json:"app_description"`

	// Connect holds the database connection configuration including DSN and credentials.
	Connect DatabaseConfig `yaml:"connect" json:"connect"`

	// ResourceTypes defines the set of resource types (e.g., user, role) configured in the connector.
	ResourceTypes map[string]ResourceType `yaml:"resource_types" json:"resource_types"`
}

// DatabaseConfig contains settings required to connect to the database.
type DatabaseConfig struct {
	// DSN is the Database Source Name connection string used to establish the database connection.
	DSN string `yaml:"dsn" json:"dsn"`

	// These fields are not required if the DSN already includes the credentials.
	// They should only be provided if the username or password contain characters that need URL encoding.

	// User is the database username used for authentication.
	User string `yaml:"user" json:"user"`

	// Password is the database password used for authentication.
	Password string `yaml:"password" json:"password"`
}

// ResourceType defines configuration for a specific type of resource.
type ResourceType struct {
	// Name is the display name for this resource type.
	Name string `yaml:"name" json:"name"`

	// List contains the configuration for querying a list of resources.
	List *ListQuery `yaml:"list,omitempty" json:"list,omitempty"`

	// Entitlements defines dynamic entitlement query and mapping settings.
	Entitlements *EntitlementsQuery `yaml:"entitlements,omitempty" json:"entitlements,omitempty"`

	// StaticEntitlements lists predefined entitlement mappings that do not require dynamic queries.
	StaticEntitlements []*EntitlementMapping `yaml:"static_entitlements,omitempty" json:"static_entitlements,omitempty"`

	// Grants defines the configuration for discovering existing entitlement grants.
	Grants []*GrantsQuery `yaml:"grants,omitempty" json:"grants,omitempty"`

	// Description provides additional information or context for the resource type.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// SkipEntitlementsAndGrants indicates if entitlement and grant processing should be bypassed.
	SkipEntitlementsAndGrants bool `yaml:"skip_entitlements_and_grants,omitempty" json:"skip_entitlements_and_grants,omitempty"`

	// AccountProvisioning defines the configuration for provisioning new accounts
	AccountProvisioning *AccountProvisioning `yaml:"account_provisioning,omitempty" json:"account_provisioning,omitempty"`
}

// ListQuery defines the structure for configuring resource list queries.
type ListQuery struct {
	// Vars provides variables that can be used within the list query.
	// Variables can reference input fields via 'input.fieldname' and credential data via 'credentials.fieldname'
	Vars map[string]string `yaml:"vars,omitempty" json:"vars,omitempty"`

	// Query is the SQL statement used to fetch a list of resources.
	Query string `yaml:"query" json:"query"`

	// Pagination defines the pagination strategy and settings for the list query.
	Pagination *Pagination `yaml:"pagination" json:"pagination"`

	// Map specifies how to map raw query columns to standardized resource fields.
	Map *ResourceMapping `yaml:"map" json:"map"`
}

// ResourceMapping defines how to map SQL query results to resource properties.
type ResourceMapping struct {
	// Id maps the SQL result column to the resource's unique identifier.
	Id string `yaml:"id" json:"id"`

	// DisplayName maps the SQL result column to the resource's human-readable name.
	DisplayName string `yaml:"display_name" json:"display_name"`

	// Description maps the SQL result column to a textual description of the resource.
	Description string `yaml:"description" json:"description"`

	// Traits defines specific attribute mappings for various resource subtypes (e.g., user, role).
	Traits *Traits `yaml:"traits" json:"traits"`

	// Annotations includes additional metadata such as entitlement immutability and external links.
	Annotations *Annotations `yaml:"annotations" json:"annotations"`
}

// Annotations holds extra metadata for resource or grant mappings.
type Annotations struct {
	// EntitlementImmutable provides settings to mark an entitlement as immutable (e.g., cannot be revoked).
	EntitlementImmutable *v2.EntitlementImmutable `yaml:"entitlement_immutable" json:"entitlement_immutable"`

	// ExternalLink provides an external URL reference related to the resource or entitlement.
	ExternalLink *v2.ExternalLink `yaml:"external_link" json:"external_link"`
}

// Traits defines attribute mappings for different resource types.
type Traits struct {
	// App contains trait mappings specific to the application level.
	App *AppTraitMapping `yaml:"app" json:"app"`

	// Group contains trait mappings for group resources.
	Group *GroupTraitMapping `yaml:"group" json:"group"`

	// Role contains trait mappings for role resources.
	Role *RoleTraitMapping `yaml:"role" json:"role"`

	// User contains trait mappings for user resources.
	User *UserTraitMapping `yaml:"user" json:"user"`
}

// UserTraitMapping defines attribute mappings specifically for user resources.
type UserTraitMapping struct {
	// Emails specifies a list of email addresses associated with the user.
	// The first email is used as the primary email address.
	Emails []string `yaml:"emails" json:"emails"`

	// Status indicates the current status of the user (e.g., active, inactive).
	// Supported values are:
	// Enabled: active, enabled
	// Disabled: disabled, inactive, suspended, locked
	// Deleted: deleted
	Status string `yaml:"status" json:"status"`

	// StatusDetails provides additional information about the user's status.
	StatusDetails string `yaml:"status_details" json:"status_details"`

	// Profile is a set of key-value pairs representing user profile attributes.
	Profile map[string]string `yaml:"profile" json:"profile"`

	// AccountType defines the type of user account.
	// Supported values are: user, human, service, system
	AccountType string `yaml:"account_type" json:"account_type"`

	// Login is the user's primary login identifier.
	Login string `yaml:"login" json:"login"`

	// LoginAliases lists alternative login identifiers for the user.
	LoginAliases []string `yaml:"login_aliases" json:"login_aliases"`

	// LastLogin records the time of the user's last login.
	LastLogin string `yaml:"last_login" json:"last_login"`

	// EmployeeIds stores the employee identifier(s) for the user.
	EmployeeIDs []string `yaml:"employee_ids" json:"employee_ids"`

	// ManagerID is the identifier of the user's manager.
	ManagerID string `yaml:"manager_id" json:"manager_id"`

	// ManagerEmail is the email address of the user's manager.
	ManagerEmail string `yaml:"manager_email" json:"manager_email"`

	// MfaEnabled indicates whether multi-factor authentication is enabled for the user.
	MfaEnabled string `yaml:"mfa_enabled" json:"mfa_enabled"`

	// SsoEnabled indicates whether single sign-on is enabled for the user.
	SsoEnabled string `yaml:"sso_enabled" json:"sso_enabled"`
}

// GroupTraitMapping defines attribute mappings for group resources.
type GroupTraitMapping struct {
	// Profile is a set of key-value pairs representing group profile attributes.
	Profile map[string]string `yaml:"profile" json:"profile"`
}

// AppTraitMapping defines attribute mappings at the application level.
type AppTraitMapping struct {
	// HelpUrl provides a link to help documentation for the application.
	HelpUrl string `yaml:"help_url" json:"help_url"`

	// Profile is a set of key-value pairs representing application profile attributes.
	Profile map[string]string `yaml:"profile" json:"profile"`
}

// RoleTraitMapping defines attribute mappings for role resources.
type RoleTraitMapping struct {
	// Profile is a set of key-value pairs representing role-specific attributes.
	Profile map[string]string `yaml:"profile" json:"profile"`
}

// Pagination defines how query results should be paginated.
type Pagination struct {
	// Strategy defines the pagination approach, e.g., "offset" or "cursor".
	Strategy string `yaml:"strategy" json:"strategy"`

	// PrimaryKey is the column used to uniquely identify records for pagination purposes.
	PrimaryKey string `yaml:"primary_key,omitempty" json:"primary_key,omitempty"`
}

// EntitlementsQuery defines the structure for querying dynamic entitlements.
type EntitlementsQuery struct {
	// Vars provides variables that can be used within the entitlements query.
	// Variables can reference input fields via 'input.fieldname' and credential data via 'credentials.fieldname'
	Vars map[string]string `yaml:"vars,omitempty" json:"vars,omitempty"`

	// Query is the SQL statement used to fetch dynamic entitlements.
	Query string `yaml:"query" json:"query"`

	// Pagination defines how pagination should be handled for the entitlements query.
	Pagination *Pagination `yaml:"pagination" json:"pagination"`

	// Map contains mappings that interpret query results as entitlement objects.
	Map []*EntitlementMapping `yaml:"map" json:"map"`
}

// EntitlementMapping defines how query results are mapped to an entitlement.
type EntitlementMapping struct {
	// Id is the unique identifier for the entitlement.
	Id string `yaml:"id" json:"id"`

	// DisplayName is the human-readable name of the entitlement.
	DisplayName string `yaml:"display_name" json:"display_name"`

	// Description provides details about what the entitlement represents.
	Description string `yaml:"description" json:"description"`

	// GrantableTo lists the resource types that are eligible to receive this entitlement.
	GrantableTo []string `yaml:"grantable_to" json:"grantable_to"`

	// Purpose indicates the intended use of the entitlement (e.g., access, assignment).
	// Supported values are: assignment, permission
	Purpose string `yaml:"purpose" json:"purpose"`

	// Slug is a short identifier, possibly used in URLs.
	Slug string `yaml:"slug" json:"slug"`

	// Immutable indicates whether this entitlement is fixed and cannot be granted or revoked.
	Immutable bool `yaml:"immutable" json:"immutable"`

	// SkipIf provides a CEL expression that evaluates to true in order to skip processing this entitlement mapping.
	SkipIf string `yaml:"skip_if" json:"skip_if"`

	// Provisioning contains the configuration for granting and revoking this entitlement.
	Provisioning *EntitlementProvisioning `yaml:"provisioning,omitempty" json:"provisioning,omitempty"`
}

// EntitlementProvisioning defines settings and queries for entitlement provisioning.
type EntitlementProvisioning struct {
	// Grant defines the SQL queries and settings for granting this entitlement.
	Grant *EntitlementProvisioningQueries `yaml:"grant,omitempty" json:"grant,omitempty"`

	// Revoke defines the SQL queries and settings for revoking this entitlement.
	Revoke *EntitlementProvisioningQueries `yaml:"revoke,omitempty" json:"revoke,omitempty"`

	// Vars provides variables that can be used within provisioning SQL queries.
	Vars map[string]string `yaml:"vars,omitempty" json:"vars,omitempty"`
}

// EntitlementProvisioningQueries defines the SQL statements used for entitlement provisioning operations.
type EntitlementProvisioningQueries struct {
	// NoTransaction indicates whether the provisioning queries should be executed without a transaction.
	NoTransaction bool `yaml:"no_transaction,omitempty" json:"no_transaction,omitempty"`

	// Queries is a list of SQL statements to execute for the provisioning operation.
	Queries []string `yaml:"queries,omitempty" json:"queries,omitempty"`
}

// GrantsQuery defines the structure for querying existing entitlement grants.
type GrantsQuery struct {
	// Vars provides variables that can be used within the grants query.
	// Variables can reference input fields via 'input.fieldname' and credential data via 'credentials.fieldname'
	Vars map[string]string `yaml:"vars,omitempty" json:"vars,omitempty"`

	// Query is the SQL statement used to retrieve existing entitlement grants.
	Query string `yaml:"query" json:"query"`

	// Pagination defines how to paginate through the results of the grants query.
	Pagination *Pagination `yaml:"pagination" json:"pagination"`

	// Map contains mappings to interpret each row of the query result as a grant.
	Map []*GrantMapping `yaml:"map" json:"map"`
}

// GrantMapping defines how query results are mapped to an entitlement grant.
type GrantMapping struct {
	// SkipIf provides a CEL expression to ignore this row mapping if the condition evaluates to true.
	SkipIf string `yaml:"skip_if" json:"skip_if"`

	// PrincipalId maps the SQL result column to the principal's unique identifier.
	PrincipalId string `yaml:"principal_id" json:"principal_id"`

	// PrincipalType maps the SQL result column to the type of principal (e.g., "user" or "group").
	PrincipalType string `yaml:"principal_type" json:"principal_type"`

	// Entitlement maps the SQL result column to the identifier of the associated entitlement.
	Entitlement string `yaml:"entitlement_id" json:"entitlement_id"`

	// Annotations includes additional metadata for the grant mapping.
	Annotations *Annotations `yaml:"annotations" json:"annotations"`

	// Expandable indicates whether the grant should be expanded.
	Expandable *ExpandableGrant `yaml:"expandable,omitempty" json:"expandable,omitempty"`
}

type ExpandableGrant struct {
	// SkipIf provides a CEL expression to ignore this row mapping if the condition evaluates to true.
	SkipIf string `yaml:"skip_if,omitempty" json:"skip_if,omitempty"`

	// Entitlements is a list of entitlement IDs to expand.
	Entitlements []string `yaml:"entitlement_ids" json:"entitlement_ids"`

	// Shallow indicates whether the grant should be expanded shallowly.
	Shallow bool `yaml:"shallow,omitempty" json:"shallow,omitempty"`
}

// AccountProvisioning defines the configuration for provisioning new accounts.
type AccountProvisioning struct {
	// Schema defines the required fields for account creation.
	Schema []*AccountProvisioningField `yaml:"schema" json:"schema"`
	// Credentials defines the supported credential handlers.
	Credentials *AccountCredentials `yaml:"credentials" json:"credentials"`
	// Create defines the SQL queries and configuration for creating new accounts.
	Create *AccountCreationConfig `yaml:"create" json:"create"`
	// Validate defines the SQL queries and configuration for validating new accounts.
	Validate *AccountValidationConfig `yaml:"validate" json:"validate"`
}

// AccountProvisioningField defines a field required for account provisioning.
type AccountProvisioningField struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Type        string `yaml:"type" json:"type"`
	Placeholder string `yaml:"placeholder,omitempty" json:"placeholder,omitempty"`
	Required    bool   `yaml:"required" json:"required"`
}

// AccountCredentials defines the supported credential handlers and their configurations.
type AccountCredentials struct {
	NoPassword     *NoPasswordConfig     `yaml:"no_password,omitempty" json:"no_password,omitempty"`
	RandomPassword *RandomPasswordConfig `yaml:"random_password,omitempty" json:"random_password,omitempty"`
}

// BaseCredentialConfig contains fields common to all credential handlers.
type BaseCredentialConfig struct {
	Preferred bool `yaml:"preferred" json:"preferred"`
}

// NoPasswordConfig defines configuration for accounts that don't require passwords.
type NoPasswordConfig struct {
	BaseCredentialConfig `yaml:",inline"`
}

// RandomPasswordConfig defines configuration for random password generation.
type RandomPasswordConfig struct {
	BaseCredentialConfig `yaml:",inline"`
	MaxLength            int    `yaml:"max_length" json:"max_length"`
	MinLength            int    `yaml:"min_length" json:"min_length"`
	DisallowedCharacters string `yaml:"disallowed_characters" json:"disallowed_characters"`
}

// AccountValidationConfig defines the configuration for validating new accounts.
type AccountValidationConfig struct {
	// Vars provides variables that can be used within account validation SQL queries.
	Vars map[string]string `yaml:"vars,omitempty" json:"vars,omitempty"`
	// Queries is a list of SQL statements to execute for account validation.
	Query string `yaml:"query" json:"queries"`
}

// AccountCreationConfig defines the configuration for creating new accounts.
type AccountCreationConfig struct {
	// Vars provides variables that can be used within account creation SQL queries.
	// Variables can reference input fields via 'input.fieldname' and credential data via 'credentials.fieldname'.
	Vars map[string]string `yaml:"vars,omitempty" json:"vars,omitempty"`
	// Queries is a list of SQL statements to execute for account creation.
	Queries []string `yaml:"queries" json:"queries"`
	// NoTransaction indicates whether the creation queries should be executed without a transaction.
	NoTransaction bool `yaml:"no_transaction,omitempty" json:"no_transaction,omitempty"`
}

func (c Config) ExtractAccountProvisioning() (string, *AccountProvisioning, error) {
	for rtID, rt := range c.ResourceTypes {
		if rt.AccountProvisioning != nil {
			return rtID, rt.AccountProvisioning, nil
		}
	}
	return "", nil, ErrNoAccountProvisioningDefined
}

// Parse converts YAML-encoded configuration data into a Config struct.
func Parse(data []byte) (*Config, error) {
	config := &Config{}
	err := yaml.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// LoadConfigFromFile reads a YAML configuration file from the given path and parses its content into a Config struct.
func LoadConfigFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	config := &Config{}
	err = yaml.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// GetAccountCreationSchema returns the account creation schema for the connector metadata.
func (c *Config) GetAccountCreationSchema(ctx context.Context) (*v2.ConnectorAccountCreationSchema, error) {
	_, accountProvisioning, err := c.ExtractAccountProvisioning()
	if err != nil {
		if errors.Is(err, ErrNoAccountProvisioningDefined) {
			return nil, nil
		}

		return nil, err
	}

	schema := &v2.ConnectorAccountCreationSchema{
		FieldMap: make(map[string]*v2.ConnectorAccountCreationSchema_Field),
	}

	for _, field := range accountProvisioning.Schema {
		schemaField := &v2.ConnectorAccountCreationSchema_Field{
			DisplayName: field.Name,
			Description: field.Description,
			Required:    field.Required,
			Placeholder: field.Placeholder,
		}

		switch field.Type {
		case "string":
			schemaField.Field = &v2.ConnectorAccountCreationSchema_Field_StringField{
				StringField: &v2.ConnectorAccountCreationSchema_StringField{},
			}

		case "string_list":
			schemaField.Field = &v2.ConnectorAccountCreationSchema_Field_StringListField{
				StringListField: &v2.ConnectorAccountCreationSchema_StringListField{},
			}

		case "boolean":
			schemaField.Field = &v2.ConnectorAccountCreationSchema_Field_BoolField{
				BoolField: &v2.ConnectorAccountCreationSchema_BoolField{},
			}

		case "int":
			schemaField.Field = &v2.ConnectorAccountCreationSchema_Field_IntField{
				IntField: &v2.ConnectorAccountCreationSchema_IntField{},
			}

		case "map":
			schemaField.Field = &v2.ConnectorAccountCreationSchema_Field_MapField{
				MapField: &v2.ConnectorAccountCreationSchema_MapField{},
			}

		default:
			return nil, fmt.Errorf("unsupported field type: %s", field.Type)
		}

		schema.FieldMap[field.Name] = schemaField
	}

	return schema, nil
}
