package bsql

import (
	"context"
	"errors"
	"fmt"
	"os"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v3"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
)

type staticValidator interface {
	staticValidate(ctx context.Context, s *SQLSyncer) error
}

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

	// Actions defines the set of actions configured in the connector.
	Actions map[string]ActionConfig `yaml:"actions" json:"actions"`
}

func (c Config) HasActions() bool {
	return len(c.Actions) > 0
}

// DatabaseConfig contains settings required to connect to the database.
// You can specify either a complete DSN, or use structured fields, or a combination.
// Structured fields override corresponding parts of the DSN when both are provided.
type DatabaseConfig struct {
	// DSN is the Database Source Name connection string (optional if using structured fields).
	// Supports environment variable expansion via ${VAR_NAME} syntax.
	// Example: "postgres://${DB_HOST}:${DB_PORT}/${DB_DATABASE}?sslmode=disable"
	DSN string `yaml:"dsn" json:"dsn"`

	// Structured connection fields (optional, override DSN components when set)

	// Scheme is the database type (e.g., "postgres", "mysql", "sqlserver", "oracle", "hdb")
	Scheme string `yaml:"scheme" json:"scheme"`

	// Host is the database server hostname or IP address (may include port for some databases)
	Host string `yaml:"host" json:"host"`

	// Port is the database server port number
	Port string `yaml:"port" json:"port"`

	// Database is the name of the database to connect to
	Database string `yaml:"database" json:"database"`

	// User is the database username used for authentication
	User string `yaml:"user" json:"user"`

	// Password is the database password used for authentication
	Password string `yaml:"password" json:"password"`

	// Params contains additional connection parameters (e.g., {"sslmode": "disable", "timeout": "30s"})
	Params map[string]string `yaml:"params" json:"params"`

	// Databases opts the connector into per-database iteration: each list/entitlements/grants
	// query runs once per named database. Leave unset for single-database connectors.
	Databases *DatabasesConfig `yaml:"databases,omitempty" json:"databases,omitempty"`
}

type DatabasesConfig struct {
	Static []string `yaml:"static,omitempty" json:"static,omitempty"`

	// DiscoveryQuery is run against an admin handle (the DSN's Database field) before
	// the per-database handles are opened; its first column is the list of database names.
	DiscoveryQuery string `yaml:"discovery_query,omitempty" json:"discovery_query,omitempty"`
}

func (d *DatabasesConfig) Validate() error {
	hasStatic := len(d.Static) > 0
	hasDiscovery := d.DiscoveryQuery != ""
	if hasStatic && hasDiscovery {
		return errors.New("databases: only one of static or discovery_query may be set")
	}
	if !hasStatic && !hasDiscovery {
		return errors.New("databases: at least one of static or discovery_query must be set")
	}
	return nil
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

	// CredentialRotation defines the configuration for credential rotation
	CredentialRotation *CredentialRotation `yaml:"credential_rotation,omitempty" json:"credential_rotation,omitempty"`
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

	// Scope = "cluster" opts a query out of per-database iteration; otherwise the query
	// runs once per database. Ignored when connect.databases is unset.
	Scope string `yaml:"scope,omitempty" json:"scope,omitempty"`
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

	// NonHumanIdentity marks the resource as a non-human identity (K3). It is
	// kind-agnostic: it attaches a NonHumanIdentityTrait annotation alongside
	// whatever primary trait (if any) the resource carries, so it lives here as
	// a sibling of Traits rather than inside it.
	NonHumanIdentity *NonHumanIdentityMapping `yaml:"non_human_identity,omitempty" json:"non_human_identity,omitempty"`

	// Annotations includes additional metadata such as entitlement immutability and external links.
	Annotations *Annotations `yaml:"annotations" json:"annotations"`
}

// NonHumanIdentityMapping declares that a resource is a non-human identity (K3).
// Both fields are CEL expressions evaluated against the query row.
type NonHumanIdentityMapping struct {
	// NhiType is the kind of non-human identity.
	// Supported values: app_registration, assumable_role, managed_identity.
	NhiType string `yaml:"nhi_type" json:"nhi_type"`

	// NhiDetail is a free-form descriptor of the identity, conventionally
	// "<platform>.<object>" (e.g. "aws.iam_role").
	NhiDetail string `yaml:"nhi_detail,omitempty" json:"nhi_detail,omitempty"`
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

	// Secret contains trait mappings for secret/credential resources (K1).
	Secret *SecretTraitMapping `yaml:"secret,omitempty" json:"secret,omitempty"`

	// Agent contains trait mappings for AI-agent resources.
	Agent *AgentTraitMapping `yaml:"agent,omitempty" json:"agent,omitempty"`
}

// SecretTraitMapping defines attribute mappings for secret/credential resources (K1).
// All fields are CEL expressions evaluated against the query row.
type SecretTraitMapping struct {
	// CredentialType classifies the secret.
	// Supported values: static_secret, asymmetric_key, certificate.
	CredentialType string `yaml:"credential_type" json:"credential_type"`

	// CredentialDetail is a free-form descriptor of the credential,
	// conventionally "<platform>.<object>" (e.g. "postgres.api_token").
	CredentialDetail string `yaml:"credential_detail,omitempty" json:"credential_detail,omitempty"`

	// ExpiresAt records when the credential expires (parsed using the DB engine's time format).
	ExpiresAt string `yaml:"expires_at,omitempty" json:"expires_at,omitempty"`

	// LastUsedAt records when the credential was last used.
	LastUsedAt string `yaml:"last_used_at,omitempty" json:"last_used_at,omitempty"`
}

// AgentTraitMapping defines attribute mappings for AI-agent resources.
// String fields are CEL expressions evaluated against the query row.
type AgentTraitMapping struct {
	// Status is the agent's lifecycle status.
	// Supported values: ready (active, enabled), disabled (inactive), deleted.
	Status string `yaml:"status,omitempty" json:"status,omitempty"`

	// IdentityResourceType is the resource type of the identity the agent
	// authenticates as. Required (together with IdentityResourceID) to set the
	// agent's identity reference.
	IdentityResourceType string `yaml:"identity_resource_type,omitempty" json:"identity_resource_type,omitempty"`

	// IdentityResourceID is the resource id of the identity the agent
	// authenticates as.
	IdentityResourceID string `yaml:"identity_resource_id,omitempty" json:"identity_resource_id,omitempty"`

	// Profile is a set of key-value pairs representing agent profile attributes.
	Profile map[string]string `yaml:"profile,omitempty" json:"profile,omitempty"`
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

	// PageSize overrides the default number of rows fetched per page (default: 100, max: 1000).
	// Reduce this value if query results are large and exceed gRPC message size limits.
	PageSize int `yaml:"page_size,omitempty" json:"page_size,omitempty"`
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

	Scope string `yaml:"scope,omitempty" json:"scope,omitempty"`
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

	// ExclusionGroup declares that this entitlement belongs to a mutually
	// exclusive group on its parent resource. Temporary shape: emitted as a
	// hand-rolled c1.connector.v2.EntitlementExclusionGroup Any annotation
	// until the upstream baton-sdk type lands and the encoding can be replaced
	// with annotations.Update(&v2.EntitlementExclusionGroup{...}).
	ExclusionGroup *ExclusionGroupMapping `yaml:"exclusion_group,omitempty" json:"exclusion_group,omitempty"`
}

// ExclusionGroupMapping is the temporary YAML shape for the
// c1.connector.v2.EntitlementExclusionGroup annotation. All three fields
// are CEL expressions evaluated against the entitlement row.
type ExclusionGroupMapping struct {
	// Id is the opaque exclusion group identifier (proto field 1, string).
	Id string `yaml:"id" json:"id"`

	// Order is an optional ordering hint within the group (proto field 2, uint32).
	Order string `yaml:"order,omitempty" json:"order,omitempty"`

	// IsDefault marks this entitlement as the group's default (proto field 3, bool).
	IsDefault string `yaml:"is_default,omitempty" json:"is_default,omitempty"`

	// ScopeToResource indicates whether to scope the exclusion group to a resource on static entitlement (proto field 4, bool).
	IsScopeToResource string `yaml:"is_scope_to_resource,omitempty" json:"is_scope_to_resource,omitempty"`
}

// EntitlementProvisioning defines settings and queries for entitlement provisioning.
type EntitlementProvisioning struct {
	// Grant defines the SQL queries and settings for granting this entitlement.
	Grant *GrantEntitlementProvisioningQueries `yaml:"grant,omitempty" json:"grant,omitempty"`

	// Revoke defines the SQL queries and settings for revoking this entitlement.
	Revoke *RevokeEntitlementProvisioningQueries `yaml:"revoke,omitempty" json:"revoke,omitempty"`

	// Vars provides variables that can be used within provisioning SQL queries.
	Vars map[string]string `yaml:"vars,omitempty" json:"vars,omitempty"`
}

// EntitlementProvisioningQueries defines the SQL statements used for entitlement provisioning operations.
type EntitlementProvisioningQueries struct {
	// NoTransaction indicates whether the provisioning queries should be executed without a transaction.
	NoTransaction bool `yaml:"no_transaction,omitempty" json:"no_transaction,omitempty"`

	// ValidationQueries is a list of SQL statements to execute for validating the provisioning operation before execution.
	ValidationQueries []string `yaml:"validation_queries,omitempty" json:"validation_queries,omitempty"`

	// Queries is a list of SQL statements to execute for the provisioning operation.
	Queries []string `yaml:"queries,omitempty" json:"queries,omitempty"`
}

// RevokeOptions holds optional revoke-only behavior beyond the shared provisioning queries.
type RevokeOptions struct {
	// PrincipalExistsCheck probes whether the principal still exists after the revoke queries run.
	// No rows means the principal was deleted as a side effect of the revoke.
	PrincipalExistsCheck *PrincipalExistsCheck `yaml:"principal_exists_check,omitempty" json:"principal_exists_check,omitempty"`
}

// PrincipalExistsCheck configures a probe query that reports whether the principal still exists after a revoke.
type PrincipalExistsCheck struct {
	// Query runs with the same provisioning vars once the revoke queries have committed.
	// Returning at least one row means the principal still exists;
	// returning no rows means it was deleted as a side effect of the revoke.
	// A query that fails does not fail the revoke; the deletion just goes unreported.
	Query string `yaml:"query" json:"query"`
}

type GrantReplaceProvisioningQueries struct {
	// Query is the SQL statement used to retrieve grant
	Query string `yaml:"query" json:"query"`
	// Map contains mappings to interpret each row of the query result as a grant.
	Map []*GrantMapping `yaml:"map" json:"map"`
}

type GrantRejectIfProvisioningQuery struct {
	// Query is the SQL statement used to determine whether to reject the grant.
	Query string `yaml:"query" json:"query"`

	// Reason is a CEL expression evaluated against the first returned row.
	Reason string `yaml:"reason" json:"reason"`
}

type GrantEntitlementProvisioningQueries struct {
	EntitlementProvisioningQueries `yaml:",inline" json:",inline"`

	// RejectIf defines a policy query that intentionally rejects the grant when it returns at least one row.
	RejectIf *GrantRejectIfProvisioningQuery `yaml:"reject_if,omitempty" json:"reject_if,omitempty"`

	// GrantReplaceProvisioningQueries defines the SQL queries and settings for replacing existing grants with the new grant during provisioning.
	GrantReplace *GrantReplaceProvisioningQueries `yaml:"grant_replace,omitempty" json:"grant_replace,omitempty"`
}

// RevokeEntitlementProvisioningQueries extends the shared provisioning query fields with revoke-only behavior.
type RevokeEntitlementProvisioningQueries struct {
	EntitlementProvisioningQueries `yaml:",inline" json:",inline"`

	// RevokeOptions groups optional revoke-only settings such as principal_exists_check.
	RevokeOptions *RevokeOptions `yaml:"revoke_options,omitempty" json:"revoke_options,omitempty"`
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

	Scope string `yaml:"scope,omitempty" json:"scope,omitempty"`
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

	// EntitlementResourceId is used for grant replace on grant action
	EntitlementResourceId string `yaml:"entitlement_resource_id" json:"entitlement_resource_id"`
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
	NoPassword        *NoPasswordConfig        `yaml:"no_password,omitempty" json:"no_password,omitempty"`
	RandomPassword    *RandomPasswordConfig    `yaml:"random_password,omitempty" json:"random_password,omitempty"`
	EncryptedPassword *EncryptedPasswordConfig `yaml:"encrypted_password,omitempty" json:"encrypted_password,omitempty"`
}

// BaseCredentialConfig contains fields common to all credential handlers.
type BaseCredentialConfig struct {
	Preferred bool `yaml:"preferred" json:"preferred"`
}

// NoPasswordConfig defines configuration for accounts that don't require passwords.
type NoPasswordConfig struct {
	BaseCredentialConfig `yaml:",inline"`
}

// PasswordConstraintConfig defines a character set constraint for random password generation.
type PasswordConstraintConfig struct {
	// CharSet is the set of characters that must appear in the generated password.
	CharSet string `yaml:"char_set" json:"char_set"`

	// MinCount is the minimum number of characters from CharSet that the password must contain.
	// Must be greater than zero.
	MinCount int `yaml:"min_count" json:"min_count"`
}

// RandomPasswordConfig defines configuration for random password generation.
type RandomPasswordConfig struct {
	BaseCredentialConfig `yaml:",inline"`

	// Deprecated: MaxLength  is not implemented and has no effect.
	// The actual password length is determined by the platform via LocalCredentialOptions.
	MaxLength int `yaml:"max_length" json:"max_length"`

	// Deprecated: MinLength is not implemented and has no effect.
	// The actual password length is determined by the platform via LocalCredentialOptions.
	MinLength int `yaml:"min_length" json:"min_length"`

	// Deprecated: DisallowedCharacters is not implemented and has no effect.
	// Use Constraints to restrict which characters appear in generated passwords.
	DisallowedCharacters string `yaml:"disallowed_characters" json:"disallowed_characters"`

	// Constraints defines the character set rules enforced when generating a random password.
	// Each entry specifies a character set and the minimum number of characters from that set
	// that must appear in the generated password. When set, these constraints replace any
	// constraints provided by the platform.
	Constraints []PasswordConstraintConfig `yaml:"constraints,omitempty" json:"constraints,omitempty"`
}

// EncryptedPasswordConfig defines configuration for encrypted password generation.
type EncryptedPasswordConfig struct {
	BaseCredentialConfig `yaml:",inline"`
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

type CredentialRotation struct {
	// Credentials defines the supported credential handlers.
	Credentials *AccountCredentials `yaml:"credentials" json:"credentials"`
	// Update defines the SQL queries and configuration for updating credentials.
	Update *AccountCreationConfig `yaml:"update" json:"update"`
}

type ActionConfig struct {
	Name          string                    `yaml:"name" json:"name" validate:"required"`
	Description   string                    `yaml:"description,omitempty" json:"description,omitempty" validate:"omitempty"`
	Arguments     map[string]ArgumentConfig `yaml:"arguments,omitempty" json:"arguments,omitempty" validate:"omitempty,dive"`
	Vars          map[string]string         `yaml:"vars,omitempty" json:"vars,omitempty" validate:"omitempty"`
	NoTransaction bool                      `yaml:"no_transaction,omitempty" json:"no_transaction,omitempty" validate:"omitempty"`
	Query         string                    `yaml:"query,omitempty" json:"query,omitempty" validate:"required_without=queries,excluded_with=queries,omitempty"`
	Queries       []string                  `yaml:"queries,omitempty" json:"queries,omitempty" validate:"required_without=query,excluded_with=query,omitempty"`
	// TODO: add validation?
	//revive:disable-next-line:line-length-limit // because it's a long field
	ActionType []string `yaml:"action_type,omitempty" json:"action_type,omitempty" validate:"omitempty,dive,oneof=unspecified dynamic account account_update_profile account_disable account_enable"`
}

type ArgumentConfig struct {
	Name        string `yaml:"name" json:"name" validate:"required"`
	Description string `yaml:"description,omitempty" json:"description,omitempty" validate:"omitempty"`
	//revive:disable-next-line:line-length-limit // because it's a long field
	Type     string `yaml:"type" json:"type" validate:"required,oneof=string boolean number string_list string_map" jsonschema:"enum=string,enum=boolean,enum=number,enum=string_list,enum=string_map"`
	Default  any    `yaml:"default,omitempty" json:"default,omitempty" validate:"omitempty"`
	Required bool   `yaml:"required,omitempty" json:"required,omitempty" validate:"omitempty"`
}

func (a *ActionConfig) Validate() error {
	if a.Query == "" && len(a.Queries) == 0 {
		return status.Errorf(codes.InvalidArgument, "query or queries is required")
	}

	if a.Query != "" && len(a.Queries) > 0 {
		return status.Errorf(codes.InvalidArgument, "cannot specify both query and queries; use one or the other")
	}

	for _, arg := range a.Arguments {
		if arg.Required && arg.Default != nil {
			return status.Errorf(codes.InvalidArgument, "action %s argument %s is required but has a default", a.Name, arg.Name)
		}
	}

	return nil
}

func (c Config) ExtractAccountProvisioning() (string, *AccountProvisioning, error) {
	for rtID, rt := range c.ResourceTypes {
		if rt.AccountProvisioning != nil {
			return rtID, rt.AccountProvisioning, nil
		}
	}
	return "", nil, ErrNoAccountProvisioningDefined
}

func (c Config) ExtractCredentialRotation() (string, *CredentialRotation, error) {
	for rtID, rt := range c.ResourceTypes {
		if rt.CredentialRotation != nil {
			return rtID, rt.CredentialRotation, nil
		}
	}
	return "", nil, ErrNoCredentialRotationDefined
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
