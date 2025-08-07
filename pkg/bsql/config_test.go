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
			input: loadExampleConfig(t, "wordpress-test"),
			validate: func(t *testing.T, c *Config) {
				require.Equal(t, "Wordpress Test", c.AppName)
				require.Equal(t, "mysql://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_DATABASE}?charset=utf8mb4&parseTime=True&loc=Local", c.Connect.DSN)

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

				// Validate account provisioning configuration
				require.NotNil(t, userResourceType.AccountProvisioning)
				require.Len(t, userResourceType.AccountProvisioning.Schema, 2)

				// Validate schema fields
				usernameField := userResourceType.AccountProvisioning.Schema[0]
				require.Equal(t, "username", usernameField.Name)
				require.Equal(t, "The username of the user", usernameField.Description)
				require.Equal(t, "string", usernameField.Type)
				require.Equal(t, "user", usernameField.Placeholder)
				require.True(t, usernameField.Required)

				emailField := userResourceType.AccountProvisioning.Schema[1]
				require.Equal(t, "email", emailField.Name)
				require.Equal(t, "The email of the user", emailField.Description)
				require.Equal(t, "string", emailField.Type)
				require.Equal(t, "user@example.com", emailField.Placeholder)
				require.True(t, emailField.Required)

				// Validate credential handlers
				require.NotNil(t, userResourceType.AccountProvisioning.Credentials)

				// Validate no_password config
				require.NotNil(t, userResourceType.AccountProvisioning.Credentials.NoPassword)
				require.False(t, userResourceType.AccountProvisioning.Credentials.NoPassword.Preferred)

				// Validate random_password config
				require.NotNil(t, userResourceType.AccountProvisioning.Credentials.RandomPassword)
				require.Equal(t, 128, userResourceType.AccountProvisioning.Credentials.RandomPassword.MaxLength)
				require.Equal(t, 12, userResourceType.AccountProvisioning.Credentials.RandomPassword.MinLength)
				require.True(t, userResourceType.AccountProvisioning.Credentials.RandomPassword.Preferred)

				// Validate account creation configuration
				require.NotNil(t, userResourceType.AccountProvisioning.Create)
				require.False(t, userResourceType.AccountProvisioning.Create.NoTransaction)

				// Validate creation vars
				require.NotNil(t, userResourceType.AccountProvisioning.Create.Vars)
				require.Equal(t, "input.username", userResourceType.AccountProvisioning.Create.Vars["username"])
				require.Equal(t, "input.email", userResourceType.AccountProvisioning.Create.Vars["email"])
				require.Equal(t, "password", userResourceType.AccountProvisioning.Create.Vars["password"])

				// Validate creation queries
				require.Len(t, userResourceType.AccountProvisioning.Create.Queries, 1)
				require.Equal(t, normalizeQueryString(`
					INSERT INTO wp_users (user_login, user_email, user_pass)
					VALUES (?<username>, ?<email>, MD5(?<password>))
				`), normalizeQueryString(userResourceType.AccountProvisioning.Create.Queries[0]))

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
				require.Equal(t, ".user_id", roleResourceType.Grants[0].Map[0].PrincipalId)
				require.Equal(t, "user", roleResourceType.Grants[0].Map[0].PrincipalType)
				require.Equal(t, "member", roleResourceType.Grants[0].Map[0].Entitlement)
				require.Equal(t, "offset", roleResourceType.Grants[0].Pagination.Strategy)
				require.Equal(t, "user_id", roleResourceType.Grants[0].Pagination.PrimaryKey)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Parsing config for test: %s", tt.name)
			c, err := Parse([]byte(tt.input))
			if err != nil {
				t.Logf("Error parsing config: %v", err)
			}
			require.NoError(t, err)
			tt.validate(t, c)
		})
	}
}
