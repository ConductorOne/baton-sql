package bsql

import (
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	sdkGrant "github.com/conductorone/baton-sdk/pkg/types/grant"
	"github.com/conductorone/baton-sql/pkg/database"
	"github.com/stretchr/testify/require"
)

// grantValidationQuery returns a row only while the membership is absent, mirroring
// the DDL-dialect pattern where the validation query is the "is there work to do?" gate.
const grantValidationQuery = `SELECT 1 FROM users u WHERE u.id = ?<principal_id> AND NOT EXISTS (SELECT 1 FROM user_roles WHERE user_id = ?<principal_id> AND role = ?<role>)`

// revokeValidationQuery returns a row only while the membership is present.
const revokeValidationQuery = `SELECT 1 FROM user_roles WHERE user_id = ?<principal_id> AND role = ?<role>`

func withValidationQueryConfig(s *SQLSyncer) {
	s.config = ResourceType{
		StaticEntitlements: []*EntitlementMapping{
			{
				Id: "member",
				Provisioning: &EntitlementProvisioning{
					Vars: map[string]string{
						"principal_id": "principal.ID",
						"role":         "resource.ID",
					},
					Grant: &GrantEntitlementProvisioningQueries{
						EntitlementProvisioningQueries: EntitlementProvisioningQueries{
							ValidationQueries: []string{grantValidationQuery},
							Queries:           []string{`INSERT INTO user_roles (user_id, role) VALUES (?<principal_id>, ?<role>)`},
						},
					},
					Revoke: &RevokeEntitlementProvisioningQueries{
						EntitlementProvisioningQueries: EntitlementProvisioningQueries{
							ValidationQueries: []string{revokeValidationQuery},
							Queries:           []string{`DELETE FROM user_roles WHERE user_id = ?<principal_id> AND role = ?<role>`},
						},
					},
				},
			},
		},
	}
}

func memberEntitlementFor(role string) *v2.Entitlement {
	roleResource := &v2.Resource{Id: &v2.ResourceId{ResourceType: "role", Resource: role}}
	principal := &v2.Resource{Id: &v2.ResourceId{ResourceType: "user", Resource: "unused"}}
	return sdkGrant.NewGrant(roleResource, "member", principal).GetEntitlement()
}

func userPrincipal(userID string) *v2.Resource {
	return &v2.Resource{Id: &v2.ResourceId{ResourceType: "user", Resource: userID}}
}

func TestGrant_ValidationNoRowsReportsAlreadyExists(t *testing.T) {
	s, db := newRevokeProvisioningTestSyncer(t)
	withValidationQueryConfig(s)
	// validation "no rows" only signals idempotency on DDL engines (Db2)
	s.dbEngine = database.DB2
	// membership already present: the grant validation query returns no rows
	seedUserWithRoles(t, db, "user-1", "admin")

	annos, err := s.Grant(t.Context(), userPrincipal("user-1"), memberEntitlementFor("admin"))
	require.NoError(t, err)

	ok, err := annos.Pick(&v2.GrantAlreadyExists{})
	require.NoError(t, err)
	require.True(t, ok)

	// the INSERT never ran, so no duplicate row was created
	require.Equal(t, 1, countRows(t, db, `SELECT COUNT(*) FROM user_roles WHERE user_id = ? AND role = ?`, "user-1", "admin"))
}

// On a non-DDL engine, validation "no rows" is a failed precondition, not idempotency:
// Grant must return an error rather than reporting GrantAlreadyExists.
func TestGrant_ValidationNoRowsOnNonDDLEngineFailsLoudly(t *testing.T) {
	s, db := newRevokeProvisioningTestSyncer(t)
	withValidationQueryConfig(s)
	// membership already present: the grant validation query returns no rows
	seedUserWithRoles(t, db, "user-1", "admin")

	annos, err := s.Grant(t.Context(), userPrincipal("user-1"), memberEntitlementFor("admin"))
	require.Error(t, err)
	require.Nil(t, annos)

	// the INSERT never ran, so no duplicate row was created
	require.Equal(t, 1, countRows(t, db, `SELECT COUNT(*) FROM user_roles WHERE user_id = ? AND role = ?`, "user-1", "admin"))
}

func TestGrant_ValidationRowsAppliesGrant(t *testing.T) {
	s, db := newRevokeProvisioningTestSyncer(t)
	withValidationQueryConfig(s)
	// user exists without the role: validation returns a row, grant proceeds
	seedUserWithRoles(t, db, "user-1")

	annos, err := s.Grant(t.Context(), userPrincipal("user-1"), memberEntitlementFor("admin"))
	require.NoError(t, err)

	ok, err := annos.Pick(&v2.GrantAlreadyExists{})
	require.NoError(t, err)
	require.False(t, ok)

	require.Equal(t, 1, countRows(t, db, `SELECT COUNT(*) FROM user_roles WHERE user_id = ? AND role = ?`, "user-1", "admin"))
}

func TestRevoke_ValidationNoRowsReportsAlreadyRevoked(t *testing.T) {
	s, _ := newRevokeProvisioningTestSyncer(t)
	withValidationQueryConfig(s)
	// validation "no rows" only signals idempotency on DDL engines (Db2)
	s.dbEngine = database.DB2
	// nothing seeded: the revoke validation query returns no rows

	annos, err := s.Revoke(t.Context(), revokeGrantFor("user-1", "admin"))
	require.NoError(t, err)

	ok, err := annos.Pick(&v2.GrantAlreadyRevoked{})
	require.NoError(t, err)
	require.True(t, ok)
}

// On a non-DDL engine, validation "no rows" is a failed precondition, not idempotency:
// Revoke must return an error rather than reporting GrantAlreadyRevoked.
func TestRevoke_ValidationNoRowsOnNonDDLEngineFailsLoudly(t *testing.T) {
	s, _ := newRevokeProvisioningTestSyncer(t)
	withValidationQueryConfig(s)
	// nothing seeded: the revoke validation query returns no rows

	annos, err := s.Revoke(t.Context(), revokeGrantFor("user-1", "admin"))
	require.Error(t, err)
	require.Nil(t, annos)
}

func TestRevoke_ValidationRowsAppliesRevoke(t *testing.T) {
	s, db := newRevokeProvisioningTestSyncer(t)
	withValidationQueryConfig(s)
	seedUserWithRoles(t, db, "user-1", "admin")

	annos, err := s.Revoke(t.Context(), revokeGrantFor("user-1", "admin"))
	require.NoError(t, err)

	ok, err := annos.Pick(&v2.GrantAlreadyRevoked{})
	require.NoError(t, err)
	require.False(t, ok)

	require.Equal(t, 0, countRows(t, db, `SELECT COUNT(*) FROM user_roles WHERE user_id = ? AND role = ?`, "user-1", "admin"))
}

// The revoke helper must map validation "no rows" onto the sentinel so the caller
// can detect idempotency with errors.Is.
func TestRunProvisioningQueriesWithExecutor_ValidationNoRowsWrapsSentinel(t *testing.T) {
	s, db := newRevokeProvisioningTestSyncer(t)
	// validation "no rows" only signals idempotency on DDL engines (Db2)
	s.dbEngine = database.DB2

	err := s.RunProvisioningQueriesWithExecutor(
		t.Context(),
		[]string{`DELETE FROM user_roles WHERE user_id = ?<principal_id>`},
		[]string{revokeValidationQuery},
		map[string]any{"principal_id": "user-1", "role": "admin"},
		db,
	)
	require.ErrorIs(t, err, ErrQueryAffectedZeroRows)
}
