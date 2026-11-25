package bsql

import (
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	sdkEntitlement "github.com/conductorone/baton-sdk/pkg/types/entitlement"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticProvisioningDoesNotFallThrough(t *testing.T) {
	for _, operation := range []string{"grant", "revoke"} {
		for _, mode := range []string{"unsupported", "unknown", "empty_static", "supported", "dynamic_only"} {
			t.Run(operation+"/"+mode, func(t *testing.T) {
				s, db := newRevokeProvisioningTestSyncer(t)
				provisioning := func(role string) *EntitlementProvisioning {
					return &EntitlementProvisioning{
						Grant: &GrantEntitlementProvisioningQueries{EntitlementProvisioningQueries: EntitlementProvisioningQueries{
							Queries: []string{`INSERT INTO user_roles (user_id, role) VALUES ('user-1', '` + role + `')`},
						}},
						Revoke: &RevokeEntitlementProvisioningQueries{EntitlementProvisioningQueries: EntitlementProvisioningQueries{
							Queries: []string{`DELETE FROM user_roles WHERE user_id = 'user-1' AND role = '` + role + `'`},
						}},
					}
				}
				static := &EntitlementMapping{Id: "viewer", DisplayName: "'Viewer'"}
				s.config.StaticEntitlements = []*EntitlementMapping{static}
				s.config.Entitlements = &EntitlementsQuery{Map: []*EntitlementMapping{{Provisioning: provisioning("admin")}}}
				switch mode {
				case "supported":
					static.Provisioning = provisioning("viewer")
				case "empty_static":
					s.config.StaticEntitlements = []*EntitlementMapping{}
				case "dynamic_only":
					s.config.StaticEntitlements = nil
				}
				resource := &v2.Resource{Id: &v2.ResourceId{ResourceType: "role", Resource: "role-1"}}
				requested := &v2.Entitlement{Id: sdkEntitlement.NewEntitlementID(resource, "viewer"), Resource: resource}
				if mode == "unsupported" || mode == "supported" {
					entitlements, _, _, err := s.Entitlements(t.Context(), resource, nil)
					require.NoError(t, err)
					require.Len(t, entitlements, 1)
					requested = entitlements[0]
				}
				if mode == "unknown" {
					requested.Id = sdkEntitlement.NewEntitlementID(resource, "unknown")
				}
				if operation == "revoke" {
					seedUserWithRoles(t, db, "user-1", "admin", "viewer")
				}
				var err error
				if operation == "grant" {
					_, err = s.Grant(t.Context(), userPrincipal("user-1"), requested)
				} else {
					_, err = s.Revoke(t.Context(), &v2.Grant{Entitlement: requested, Principal: userPrincipal("user-1")})
				}
				supported := mode == "supported" || mode == "dynamic_only"
				if supported {
					assert.NoError(t, err)
				} else {
					assert.Error(t, err)
				}
				wantAdmin, wantViewer := 0, 0
				if operation == "revoke" {
					wantAdmin, wantViewer = 1, 1
				}
				if mode == "supported" {
					wantViewer = 1 - wantViewer
				}
				if mode == "dynamic_only" {
					wantAdmin = 1 - wantAdmin
				}
				assert.Equal(t, wantAdmin, countRows(t, db, `SELECT COUNT(*) FROM user_roles WHERE role = 'admin'`))
				assert.Equal(t, wantViewer, countRows(t, db, `SELECT COUNT(*) FROM user_roles WHERE role = 'viewer'`))
			})
		}
	}
}
