package bsql

import (
	"fmt"
	"os"
	"strings"
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/stretchr/testify/require"
)

func loadExampleConfig(t *testing.T, exampleName string) string {
	f, err := os.ReadFile(fmt.Sprintf("../../examples/%s.yml", exampleName))
	require.NoError(t, err)
	return string(f)
}

func normalizeQueryString(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Assuming Parse is a function that takes a YAML byte array and parses it into a Config struct.
func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		validate func(t *testing.T, c *Config)
	}{
		{
			name:  "wordpress-example",
			input: loadExampleConfig(t, "wordpress"),
			validate: func(t *testing.T, c *Config) {
				require.Equal(t, "Wordpress", c.AppName)
				require.Equal(t, "mysql://${DB_USERNAME}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?charset=utf8mb4&parseTime=True&loc=Local", c.Connect.DSN)

				require.Len(t, c.ResourceTypes, 2)

				// Validate `user` resource type
				userResourceType := c.ResourceTypes["user"]
				require.NotNil(t, userResourceType.List)
				require.Equal(t, "User", userResourceType.Name)
				require.Equal(t, "A user within the wordpress system", userResourceType.Description)
				require.Equal(t, normalizeQueryString(`SELECT
          u.ID AS user_id,
          u.user_login AS username,
          u.user_email AS email,
          u.user_registered AS created_at
        FROM wp_users u
        LIMIT ?<Limit> OFFSET ?<Offset>`), normalizeQueryString(userResourceType.List.Query))
				require.Equal(t, ".user_id", userResourceType.List.Map.Id)
				require.Equal(t, ".username", userResourceType.List.Map.DisplayName)
				require.Equal(t, ".email", userResourceType.List.Map.Description)
				require.Equal(t, ".email", userResourceType.List.Map.Traits.User.Emails[0].Address)
				require.Equal(t, v2.UserTrait_Status_Status(0), userResourceType.List.Map.Traits.User.Status.Status)
				require.Equal(t, "active", userResourceType.List.Map.Traits.User.Status.Details)
				require.Equal(t, ".username", userResourceType.List.Map.Traits.User.Login)

				require.Equal(t, "offset", userResourceType.List.Pagination.Strategy)
				require.Equal(t, "ID", userResourceType.List.Pagination.PrimaryKey)

				// Validate `role` resource type
				roleResourceType := c.ResourceTypes["role"]
				require.NotNil(t, roleResourceType.List)
				require.Equal(t, "Role", roleResourceType.Name)
				require.Equal(t, "A role within the wordpress system that can be assigned to a user", roleResourceType.Description)
				require.Equal(t, normalizeQueryString(`SELECT
          um.meta_value AS role_name,
          u.user_login AS username
        FROM wp_usermeta um
        JOIN wp_users u ON um.user_id = u.ID
        WHERE um.meta_key = 'wp_capabilities'
        LIMIT ?<Limit> OFFSET ?<Offset>`), normalizeQueryString(roleResourceType.List.Query))
				require.Equal(t, ".role_name", roleResourceType.List.Map.Id)
				require.Equal(t, ".role_name", roleResourceType.List.Map.DisplayName)
				require.Equal(t, "'Wordpress role for user'", roleResourceType.List.Map.Description)
				require.Equal(t, "offset", roleResourceType.List.Pagination.Strategy)
				require.Equal(t, "meta_value", roleResourceType.List.Pagination.PrimaryKey)

				// Validate `roleResourceType` entitlements
				require.NotNil(t, roleResourceType.Entitlements)
				require.Equal(t, ".user_id", roleResourceType.Entitlements.Map.Id)
				require.Equal(t, ".username", roleResourceType.Entitlements.Map.DisplayName)
				require.Equal(t, "'Role entitlement for user'", roleResourceType.Entitlements.Map.Description)
				require.Equal(t, []string{"user"}, roleResourceType.Entitlements.Map.GrantableTo)
				require.Equal(t, "offset", roleResourceType.Entitlements.Pagination.Strategy)
				require.Equal(t, "ID", roleResourceType.Entitlements.Pagination.PrimaryKey)

				// Validate `roleResourceType` grants
				require.NotNil(t, roleResourceType.Grants)
				require.Equal(t, ".user_id", roleResourceType.Grants.Map.PrincipalId)
				require.Equal(t, "'user'", roleResourceType.Grants.Map.PrincipalType)
				require.Equal(t, ".role_name", roleResourceType.Grants.Map.Entitlement)
				require.Equal(t, "offset", roleResourceType.Grants.Pagination.Strategy)
				require.Equal(t, "ID", roleResourceType.Grants.Pagination.PrimaryKey)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := Parse([]byte(tt.input))
			require.NoError(t, err)
			tt.validate(t, c)
		})
	}
}
