package bsql

import (
	"os"

	"gopkg.in/yaml.v3"

	connector_v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
)

type Config struct {
	AppName        string                  `yaml:"app_name" json:"app_name"`
	AppDescription string                  `yaml:"app_description" json:"app_description"`
	Connect        DatabaseConfig          `yaml:"connect" json:"connect"`
	ResourceTypes  map[string]ResourceType `yaml:"resource_types" json:"resource_types"`
}

type DatabaseConfig struct {
	DSN string `yaml:"dsn" json:"dsn"` // DSN connection string
}

type ResourceType struct {
	Name                      string                `yaml:"name" json:"name"`
	List                      *ListQuery            `yaml:"list,omitempty" json:"list,omitempty"`
	Entitlements              *EntitlementsQuery    `yaml:"entitlements,omitempty" json:"entitlements,omitempty"`
	StaticEntitlements        []*EntitlementMapping `yaml:"static_entitlements,omitempty" json:"static_entitlements,omitempty"`
	Grants                    []*GrantsQuery        `yaml:"grants,omitempty" json:"grants,omitempty"`
	Description               string                `yaml:"description,omitempty" json:"description,omitempty"`
	SkipEntitlementsAndGrants bool                  `yaml:"skip_entitlements_and_grants,omitempty" json:"skip_entitlements_and_grants,omitempty"`
}

type ListQuery struct {
	Query      string           `yaml:"query" json:"query"`
	Pagination *Pagination      `yaml:"pagination" json:"pagination"`
	Map        *ResourceMapping `yaml:"map" json:"map"`
}

type ResourceMapping struct {
	Id          string       `yaml:"id" json:"id"`
	DisplayName string       `yaml:"display_name" json:"display_name"`
	Description string       `yaml:"description" json:"description"`
	Traits      *Traits      `yaml:"traits" json:"traits"`
	Annotations *Annotations `yaml:"annotations" json:"annotations"`
}

type Annotations struct {
	EntitlementImmutable *connector_v2.EntitlementImmutable `yaml:"entitlement_immutable" json:"entitlement_immutable"`
	ExternalLink         *connector_v2.ExternalLink         `yaml:"external_link" json:"external_link"`
}

type Traits struct {
	App   *AppTraitMapping   `yaml:"app" json:"app"`
	Group *GroupTraitMapping `yaml:"group" json:"group"`
	Role  *RoleTraitMapping  `yaml:"role" json:"role"`
	User  *UserTraitMapping  `yaml:"user" json:"user"`
}

type UserTraitMapping struct {
	Emails        []string          `yaml:"emails" json:"emails"`
	Status        string            `yaml:"status" json:"status"`
	StatusDetails string            `yaml:"status_details" json:"status_details"`
	Profile       map[string]string `yaml:"profile" json:"profile"`
	AccountType   string            `yaml:"account_type" json:"account_type"`
	Login         string            `yaml:"login" json:"login"`
	LoginAliases  []string          `yaml:"login_aliases" json:"login_aliases"`
	LastLogin     string            `yaml:"last_login" json:"last_login"`
	MfaEnabled    string            `yaml:"mfa_enabled" json:"mfa_enabled"`
	SsoEnabled    string            `yaml:"sso_enabled" json:"sso_enabled"`
}

type GroupTraitMapping struct {
	Profile map[string]string `yaml:"profile" json:"profile"`
}

type AppTraitMapping struct {
	HelpUrl string            `yaml:"help_url" json:"help_url"`
	Profile map[string]string `yaml:"profile" json:"profile"`
}

type RoleTraitMapping struct {
	Profile map[string]string `yaml:"profile" json:"profile"`
}

type Pagination struct {
	Strategy   string `yaml:"strategy" json:"strategy"` // "offset" or "cursor"
	PrimaryKey string `yaml:"primary_key,omitempty" json:"primary_key,omitempty"`
}

type EntitlementsQuery struct {
	Query      string                `yaml:"query" json:"query"`
	Pagination *Pagination           `yaml:"pagination" json:"pagination"`
	Map        []*EntitlementMapping `yaml:"map" json:"map"`
}

type EntitlementMapping struct {
	Id          string   `yaml:"id" json:"id"`
	DisplayName string   `yaml:"display_name" json:"display_name"`
	Description string   `yaml:"description" json:"description"`
	GrantableTo []string `yaml:"grantable_to" json:"grantable_to"`
	Purpose     string   `yaml:"purpose" json:"purpose"`
	Slug        string   `yaml:"slug" json:"slug"`
	Immutable   bool     `yaml:"immutable" json:"immutable"`
	SkipIf      string   `yaml:"skip_if" json:"skip_if"`
}

type GrantsQuery struct {
	Query      string          `yaml:"query" json:"query"`
	Pagination *Pagination     `yaml:"pagination" json:"pagination"`
	Map        []*GrantMapping `yaml:"map" json:"map"`
}

type GrantMapping struct {
	SkipIf        string       `yaml:"skip_if" json:"skip_if"`
	PrincipalId   string       `yaml:"principal_id" json:"principal_id"`
	PrincipalType string       `yaml:"principal_type" json:"principal_type"`
	Entitlement   string       `yaml:"entitlement_id" json:"entitlement_id"`
	Annotations   *Annotations `yaml:"annotations" json:"annotations"`
}

func Parse(data []byte) (*Config, error) {
	config := &Config{}
	err := yaml.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

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
