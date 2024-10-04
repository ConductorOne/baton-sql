package bsql

import (
	"strings"
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/stretchr/testify/require"
)

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
			name: "wordpress-example",
			input: `
app_name: Wordpress
connect:
  dsn: "mysql://${DB_USERNAME}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?charset=utf8mb4&parseTime=True&loc=Local"
resource_types:
  users:
    list:
      query: |
        SELECT
          u.ID AS user_id,
          u.user_login AS username,
          u.user_email AS email,
          u.user_registered AS created_at
        FROM wp_users u
        LIMIT ?<Limit> OFFSET ?<Offset>
      map:
        id: ".user_id"
        display_name: ".username"
        description: ".email"
        traits:
          user:
            emails:
              - address: ".email"
            status:
              status: 0
              details: "active"
            login: ".username"
      pagination:
        strategy: "offset"
        primary_key: "ID"

  roles:
    list:
      query: |
        SELECT
          um.meta_value AS role_name,
          u.user_login AS username
        FROM wp_usermeta um
        JOIN wp_users u ON um.user_id = u.ID
        WHERE um.meta_key = 'wp_capabilities'
        LIMIT ?<Limit> OFFSET ?<Offset>
      map:
        id: ".role_name"
        display_name: ".role_name"
        description: "'Wordpress role for user'"
      pagination:
        strategy: "offset"
        primary_key: "meta_value"

    entitlements:
      query: |
        SELECT
          u.ID AS user_id,
          u.user_login AS username,
          um.meta_value AS role_name
        FROM wp_users u
        JOIN wp_usermeta um ON u.ID = um.user_id
        WHERE um.meta_key = 'wp_capabilities'
        LIMIT ?<Limit> OFFSET ?<Offset>
      map:
        id: ".user_id"
        display_name: ".username"
        description: "'Role entitlement for user'"
        grantable_to:
          - "users"
        annotations:
          entitlement_immutable:
            value: true
      pagination:
        strategy: "offset"
        primary_key: "ID"

    grants:
      query: |
        SELECT
          u.ID AS user_id,
          u.user_login AS username,
          um.meta_value AS role_name
        FROM wp_users u
        JOIN wp_usermeta um ON u.ID = um.user_id
        WHERE um.meta_key = 'wp_capabilities'
        LIMIT ?<Limit> OFFSET ?<Offset>
      map:
        principal_id: ".user_id"
        principal_type: "'user'"
        entitlement_id: ".role_name"
        annotations:
          entitlement_immutable:
            value: true
      pagination:
        strategy: "offset"
        primary_key: "ID"
`,
			validate: func(t *testing.T, c *Config) {
				require.Equal(t, "Wordpress", c.AppName)
				require.Equal(t, "mysql://${DB_USERNAME}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?charset=utf8mb4&parseTime=True&loc=Local", c.Connect.DSN)

				require.Len(t, c.ResourceTypes, 2)

				// Validate `users` resource type
				users := c.ResourceTypes["users"]
				require.NotNil(t, users.List)
				require.Equal(t, normalizeQueryString(`SELECT
          u.ID AS user_id,
          u.user_login AS username,
          u.user_email AS email,
          u.user_registered AS created_at
        FROM wp_users u
        LIMIT ?<Limit> OFFSET ?<Offset>`), normalizeQueryString(users.List.Query))
				require.Equal(t, ".user_id", users.List.Map.Id)
				require.Equal(t, ".username", users.List.Map.DisplayName)
				require.Equal(t, ".email", users.List.Map.Description)
				require.Equal(t, ".email", users.List.Map.Traits.User.Emails[0].Address)
				require.Equal(t, v2.UserTrait_Status_Status(0), users.List.Map.Traits.User.Status.Status)
				require.Equal(t, "active", users.List.Map.Traits.User.Status.Details)
				require.Equal(t, ".username", users.List.Map.Traits.User.Login)

				require.Equal(t, "offset", users.List.Pagination.Strategy)
				require.Equal(t, "ID", users.List.Pagination.PrimaryKey)

				// Validate `roles` resource type
				roles := c.ResourceTypes["roles"]
				require.NotNil(t, roles.List)
				require.Equal(t, normalizeQueryString(`SELECT
          um.meta_value AS role_name,
          u.user_login AS username
        FROM wp_usermeta um
        JOIN wp_users u ON um.user_id = u.ID
        WHERE um.meta_key = 'wp_capabilities'
        LIMIT ?<Limit> OFFSET ?<Offset>`), normalizeQueryString(roles.List.Query))
				require.Equal(t, ".role_name", roles.List.Map.Id)
				require.Equal(t, ".role_name", roles.List.Map.DisplayName)
				require.Equal(t, "'Wordpress role for user'", roles.List.Map.Description)
				require.Equal(t, "offset", roles.List.Pagination.Strategy)
				require.Equal(t, "meta_value", roles.List.Pagination.PrimaryKey)

				// Validate `roles` entitlements
				require.NotNil(t, roles.Entitlements)
				require.Equal(t, ".user_id", roles.Entitlements.Map.Id)
				require.Equal(t, ".username", roles.Entitlements.Map.DisplayName)
				require.Equal(t, "'Role entitlement for user'", roles.Entitlements.Map.Description)
				require.Equal(t, []string{"users"}, roles.Entitlements.Map.GrantableTo)
				require.Equal(t, "offset", roles.Entitlements.Pagination.Strategy)
				require.Equal(t, "ID", roles.Entitlements.Pagination.PrimaryKey)

				// Validate `roles` grants
				require.NotNil(t, roles.Grants)
				require.Equal(t, ".user_id", roles.Grants.Map.PrincipalId)
				require.Equal(t, "'user'", roles.Grants.Map.PrincipalType)
				require.Equal(t, ".role_name", roles.Grants.Map.Entitlement)
				require.Equal(t, "offset", roles.Grants.Pagination.Strategy)
				require.Equal(t, "ID", roles.Grants.Pagination.PrimaryKey)
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
