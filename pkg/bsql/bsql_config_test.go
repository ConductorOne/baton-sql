package bsql

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "wordpress-example",
			input: `
---
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
        id: ".user_id"             # Mapping user ID from query result to ID
        display_name: ".username"  # Mapping user_login to display name
        description: ".email"      # Using user email as description
        traits:
          user:
            emails:
              - address: ".email"          # Mapping user email to traits
            status:
                status: 0
                details: "active"    # Static status for users
            login: ".username"    # Mapping login (user_login)
    pagination:
      strategy: "offset"          # Using offset-based pagination
      primary_key: "ID"           # Primary key used for pagination

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
        id: ".role_name"          # Using role_name as the ID
        display_name: ".role_name" # Using role name as the display name
        description: "'Wordpress role for user'" # Static description
        traits:
          role:
            name: ".role_name"    # Mapping role name into traits
    pagination:
      strategy: "offset"
      primary_key: "meta_value"

  entitlements:
    list:
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
        id: ".user_id"            # Mapping user ID to entitlement ID
        display_name: ".username"  # Using user_login as display name
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
    list:
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
        principal_id: ".user_id"       # User ID as principal ID
        principal_type: "'user'"       # Static principal type (users)
        entitlement: ".role_name"      # Role entitlement for user
        annotations:
          entitlement_immutable:
            value: true
    pagination:
      strategy: "offset"
      primary_key: "ID"
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := Parse([]byte(tt.input))
			require.NoError(t, err)
			spew.Dump(c)
		})
	}
}
