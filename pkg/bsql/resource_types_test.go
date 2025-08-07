package bsql

import (
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/stretchr/testify/require"
)

func TestConfig_GetResourceTypes_wordpress(t *testing.T) {
	ctx := t.Context()
	wordpressConfig := loadExampleConfig(t, "wordpress-test")
	c, err := Parse([]byte(wordpressConfig))
	require.NoError(t, err)

	ret, err := c.GetResourceTypes(ctx)
	require.NoError(t, err)
	require.Len(t, ret, 2)
	for _, rt := range ret {
		switch rt.Id {
		case "user":
			require.Equal(t, "User", rt.DisplayName)
			require.Equal(t, "A user within the wordpress system", rt.Description)
			require.Len(t, rt.Traits, 1)
			require.Equal(t, v2.ResourceType_TRAIT_USER, rt.Traits[0])
		case "role":
			require.Equal(t, "Role", rt.DisplayName)
			require.Equal(t, "A role within the wordpress system that can be assigned to a user", rt.Description)
			require.Len(t, rt.Traits, 1)
			require.Equal(t, v2.ResourceType_TRAIT_ROLE, rt.Traits[0])
		default:
			require.Fail(t, "unexpected resource type")
		}
	}
}
