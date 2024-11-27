package bsql

import (
	"fmt"
	"os"
	"strings"
	"testing"

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
		ORDER BY user_id ASC
        LIMIT ?<Limit> OFFSET ?<Offset>`), normalizeQueryString(userResourceType.List.Query))
				require.Equal(t, ".user_id", userResourceType.List.Map.Id)
				require.Equal(t, ".username", userResourceType.List.Map.DisplayName)
				require.Equal(t, ".email", userResourceType.List.Map.Description)
				require.Equal(t, ".email", userResourceType.List.Map.Traits.User.Emails[0])
				require.Equal(t, "active", userResourceType.List.Map.Traits.User.Status)
				require.Equal(t, `'detailed status'`, userResourceType.List.Map.Traits.User.StatusDetails)
				require.Equal(t, ".username", userResourceType.List.Map.Traits.User.Login)

				require.Equal(t, "offset", userResourceType.List.Pagination.Strategy)
				require.Equal(t, "user_id", userResourceType.List.Pagination.PrimaryKey)

				// Validate `role` resource type
				roleResourceType := c.ResourceTypes["role"]
				require.NotNil(t, roleResourceType.List)
				require.Equal(t, "Role", roleResourceType.Name)
				require.Equal(t, "A role within the wordpress system that can be assigned to a user", roleResourceType.Description)
				require.Equal(t, normalizeQueryString(`SELECT DISTINCT
		um.umeta_id AS row_id,
		um.meta_value AS role_name
		FROM wp_usermeta um 
		WHERE
			um.meta_key = 'wp_capabilities' AND
			um.meta_value != 'a:0:{}' AND
			um.umeta_id > ?<Cursor>
		ORDER BY row_id ASC
		LIMIT ?<Limit>
`), normalizeQueryString(roleResourceType.List.Query))
				require.Equal(t, "phpDeserializeStringArray(string(.role_name))[0]", roleResourceType.List.Map.Id)
				require.Equal(t, "titleCase(phpDeserializeStringArray(string(.role_name))[0])", roleResourceType.List.Map.DisplayName)
				require.Equal(t, "'Wordpress role for user'", roleResourceType.List.Map.Description)
				require.Equal(t, "cursor", roleResourceType.List.Pagination.Strategy)
				require.Equal(t, "row_id", roleResourceType.List.Pagination.PrimaryKey)

				// Validate `roleResourceType` entitlements
				require.NotNil(t, roleResourceType.StaticEntitlements)
				require.Len(t, roleResourceType.StaticEntitlements, 1)
				require.Equal(t, "member", roleResourceType.StaticEntitlements[0].Id)
				require.Equal(t, "resource.DisplayName + ' Role Member'", roleResourceType.StaticEntitlements[0].DisplayName)
				require.Equal(t, "'Member of the ' + resource.DisplayName + ' role'", roleResourceType.StaticEntitlements[0].Description)
				require.Len(t, roleResourceType.StaticEntitlements[0].GrantableTo, 1)
				require.Equal(t, []string{"user"}, roleResourceType.StaticEntitlements[0].GrantableTo)

				// Validate `roleResourceType` grants
				require.NotNil(t, roleResourceType.Grants)
				require.Len(t, roleResourceType.Grants, 1)
				require.Equal(t, ".user_id", roleResourceType.Grants[0].Map.PrincipalId)
				require.Equal(t, "user", roleResourceType.Grants[0].Map.PrincipalType)
				require.Equal(t, "member", roleResourceType.Grants[0].Map.Entitlement)
				require.Equal(t, "offset", roleResourceType.Grants[0].Pagination.Strategy)
				require.Equal(t, "user_id", roleResourceType.Grants[0].Pagination.PrimaryKey)
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
