package bsql

import (
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestUserSyncer_prepareQueryInputs_KeyCollision(t *testing.T) {
	tests := []struct {
		name              string
		schema            []*AccountProvisioningField
		profileFields     map[string]*structpb.Value
		credentialOptions *v2.CredentialOptions
		expectInputKey    bool
		expectCredKey     bool
		expectedInputVal  any
		expectedCredVal   any
	}{
		{
			name: "No collision - normal schema fields",
			schema: []*AccountProvisioningField{
				{Name: "username", Type: "string"},
				{Name: "email", Type: "string"},
			},
			profileFields: map[string]*structpb.Value{
				"username": structpb.NewStringValue("test_user"),
				"email":    structpb.NewStringValue("test@example.com"),
			},
			credentialOptions: &v2.CredentialOptions{
				Options: &v2.CredentialOptions_RandomPassword_{
					RandomPassword: &v2.CredentialOptions_RandomPassword{
						Length: 12,
					},
				},
			},
			expectInputKey: true,
			expectCredKey:  true,
		},
		{
			name: "Collision with 'input' field in schema",
			schema: []*AccountProvisioningField{
				{Name: "input", Type: "string"},
				{Name: "username", Type: "string"},
			},
			profileFields: map[string]*structpb.Value{
				"input":    structpb.NewStringValue("user_defined_input"),
				"username": structpb.NewStringValue("test_user"),
			},
			credentialOptions: &v2.CredentialOptions{
				Options: &v2.CredentialOptions_RandomPassword_{
					RandomPassword: &v2.CredentialOptions_RandomPassword{
						Length: 12,
					},
				},
			},
			expectInputKey:   true,
			expectCredKey:    true,
			expectedInputVal: "user_defined_input",
		},
		{
			name: "Collision with 'credentials' field in schema",
			schema: []*AccountProvisioningField{
				{Name: "credentials", Type: "string"},
				{Name: "username", Type: "string"},
			},
			profileFields: map[string]*structpb.Value{
				"credentials": structpb.NewStringValue("user_defined_creds"),
				"username":    structpb.NewStringValue("test_user"),
			},
			credentialOptions: &v2.CredentialOptions{
				Options: &v2.CredentialOptions_RandomPassword_{
					RandomPassword: &v2.CredentialOptions_RandomPassword{
						Length: 12,
					},
				},
			},
			expectInputKey:  true,
			expectCredKey:   true,
			expectedCredVal: "user_defined_creds",
		},
		{
			name: "Collision with both 'input' and 'credentials' fields",
			schema: []*AccountProvisioningField{
				{Name: "input", Type: "string"},
				{Name: "credentials", Type: "string"},
				{Name: "username", Type: "string"},
			},
			profileFields: map[string]*structpb.Value{
				"input":       structpb.NewStringValue("user_defined_input"),
				"credentials": structpb.NewStringValue("user_defined_creds"),
				"username":    structpb.NewStringValue("test_user"),
			},
			credentialOptions: &v2.CredentialOptions{
				Options: &v2.CredentialOptions_RandomPassword_{
					RandomPassword: &v2.CredentialOptions_RandomPassword{
						Length: 12,
					},
				},
			},
			expectInputKey:   true,
			expectCredKey:    true,
			expectedInputVal: "user_defined_input",
			expectedCredVal:  "user_defined_creds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create SQLSyncer instance
			syncer := &SQLSyncer{}

			// Create provisioning config
			provisioningConfig := &AccountProvisioning{
				Schema: tt.schema,
			}

			// Create account info with profile fields
			accountInfo := &v2.AccountInfo{
				Profile: &structpb.Struct{
					Fields: tt.profileFields,
				},
			}

			// Call the method under test
			queryInputs, _, err := syncer.prepareQueryInputs(
				provisioningConfig,
				accountInfo,
				tt.credentialOptions,
			)

			require.NoError(t, err)
			require.NotNil(t, queryInputs)

			require.Contains(t, queryInputs, "username")
			require.Equal(t, "test_user", queryInputs["username"])

			if tt.expectInputKey {
				require.Contains(t, queryInputs, "input")
				if tt.expectedInputVal != nil {
					require.Equal(t, tt.expectedInputVal, queryInputs["input"])
				} else {
					inputMap, ok := queryInputs["input"].(map[string]any)
					require.True(t, ok, "input should be a map when not overridden")
					require.Contains(t, inputMap, "username")
					require.Equal(t, "test_user", inputMap["username"])
				}
			}

			if tt.expectCredKey {
				require.Contains(t, queryInputs, "credentials")
				if tt.expectedCredVal != nil {
					require.Equal(t, tt.expectedCredVal, queryInputs["credentials"])
				} else {
					credMap, ok := queryInputs["credentials"].(map[string]any)
					require.True(t, ok, "credentials should be a map when not overridden")
					require.Contains(t, credMap, "password")
				}
			}

			if tt.credentialOptions != nil {
				require.Contains(t, queryInputs, "password")
			}
		})
	}
}
