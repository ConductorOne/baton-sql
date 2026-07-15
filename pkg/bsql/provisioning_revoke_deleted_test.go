package bsql

import (
	"database/sql"
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	sdkGrant "github.com/conductorone/baton-sdk/pkg/types/grant"
	"github.com/conductorone/baton-sql/pkg/bcel"
	"github.com/conductorone/baton-sql/pkg/database"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// revokeDeletesUserQueries removes the role membership, then deletes the user
// row when they have no remaining roles.
var revokeDeletesUserQueries = []string{
	`DELETE FROM user_roles WHERE user_id = ?<principal_id> AND role = ?<role>`,
	`DELETE FROM users WHERE id = ?<principal_id> AND NOT EXISTS (SELECT 1 FROM user_roles WHERE user_id = ?<principal_id>)`,
}

func principalExistsCheck() *PrincipalDeletedCheck {
	return &PrincipalDeletedCheck{Query: `SELECT 1 FROM users WHERE id = ?<principal_id>`}
}

func newRevokeProvisioningTestSyncer(t *testing.T) (*SQLSyncer, *sql.DB) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	_, err = db.ExecContext(t.Context(), `CREATE TABLE users (id TEXT PRIMARY KEY)`)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `CREATE TABLE user_roles (user_id TEXT, role TEXT)`)
	require.NoError(t, err)

	env, err := bcel.NewEnv(t.Context())
	require.NoError(t, err)

	return &SQLSyncer{
		db:            db,
		dbs:           map[string]*sql.DB{"primary": db},
		dbNames:       []string{"primary"},
		primaryDBName: "primary",
		currentDBName: "primary",
		dbEngine:      database.SQLite,
		env:           env,
	}, db
}

func seedUserWithRoles(t *testing.T, db *sql.DB, userID string, roles ...string) {
	t.Helper()
	_, err := db.ExecContext(t.Context(), `INSERT INTO users (id) VALUES (?)`, userID)
	require.NoError(t, err)
	for _, r := range roles {
		_, err := db.ExecContext(t.Context(), `INSERT INTO user_roles (user_id, role) VALUES (?, ?)`, userID, r)
		require.NoError(t, err)
	}
}

func countRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&count))
	return count
}

func TestRunRevokeProvisioning_LastRoleDeletesPrincipal(t *testing.T) {
	s, db := newRevokeProvisioningTestSyncer(t)
	seedUserWithRoles(t, db, "user-1", "admin")

	deleted, err := s.RunRevokeProvisioning(
		t.Context(),
		revokeDeletesUserQueries,
		nil,
		principalExistsCheck(),
		map[string]any{"principal_id": "user-1", "role": "admin"},
		true,
	)
	require.NoError(t, err)
	require.True(t, deleted)

	require.Equal(t, 0, countRows(t, db, `SELECT COUNT(*) FROM users WHERE id = ?`, "user-1"))
	require.Equal(t, 0, countRows(t, db, `SELECT COUNT(*) FROM user_roles WHERE user_id = ?`, "user-1"))
}

func TestRunRevokeProvisioning_KeepsPrincipalWhenOtherRolesRemain(t *testing.T) {
	s, db := newRevokeProvisioningTestSyncer(t)
	seedUserWithRoles(t, db, "user-1", "admin", "viewer")

	deleted, err := s.RunRevokeProvisioning(
		t.Context(),
		revokeDeletesUserQueries,
		nil,
		principalExistsCheck(),
		map[string]any{"principal_id": "user-1", "role": "admin"},
		true,
	)
	require.NoError(t, err)
	require.False(t, deleted)

	require.Equal(t, 1, countRows(t, db, `SELECT COUNT(*) FROM users WHERE id = ?`, "user-1"))
	// only the viewer role remains
	require.Equal(t, 1, countRows(t, db, `SELECT COUNT(*) FROM user_roles WHERE user_id = ?`, "user-1"))
	require.Equal(t, 1, countRows(t, db, `SELECT COUNT(*) FROM user_roles WHERE user_id = ? AND role = ?`, "user-1", "viewer"))
}

func TestRunRevokeProvisioning_AllZeroRowsWithDeletedPrincipal(t *testing.T) {
	s, _ := newRevokeProvisioningTestSyncer(t)
	// nothing seeded: the user and role are already gone

	deleted, err := s.RunRevokeProvisioning(
		t.Context(),
		revokeDeletesUserQueries,
		nil,
		principalExistsCheck(),
		map[string]any{"principal_id": "user-1", "role": "admin"},
		true,
	)
	require.ErrorIs(t, err, ErrQueryAffectedZeroRows)
	require.True(t, deleted)
}

func TestRunRevokeProvisioning_AllZeroRowsWithSurvivingPrincipal(t *testing.T) {
	s, db := newRevokeProvisioningTestSyncer(t)
	// user still has another role, and never had the role being revoked
	seedUserWithRoles(t, db, "user-1", "viewer")

	deleted, err := s.RunRevokeProvisioning(
		t.Context(),
		revokeDeletesUserQueries,
		nil,
		principalExistsCheck(),
		map[string]any{"principal_id": "user-1", "role": "admin"},
		true,
	)
	require.ErrorIs(t, err, ErrQueryAffectedZeroRows)
	require.False(t, deleted)

	require.Equal(t, 1, countRows(t, db, `SELECT COUNT(*) FROM users WHERE id = ?`, "user-1"))
}

func TestRunRevokeProvisioning_NoDeletedCheckBehavesLikeBefore(t *testing.T) {
	s, db := newRevokeProvisioningTestSyncer(t)
	seedUserWithRoles(t, db, "user-1", "admin")

	// happy path: revoke a held role, no check configured
	deleted, err := s.RunRevokeProvisioning(
		t.Context(),
		[]string{`DELETE FROM user_roles WHERE user_id = ?<principal_id> AND role = ?<role>`},
		nil,
		nil,
		map[string]any{"principal_id": "user-1", "role": "admin"},
		true,
	)
	require.NoError(t, err)
	require.False(t, deleted)
	require.Equal(t, 0, countRows(t, db, `SELECT COUNT(*) FROM user_roles WHERE user_id = ?`, "user-1"))

	// zero-row path still returns ErrQueryAffectedZeroRows with deleted=false
	deleted, err = s.RunRevokeProvisioning(
		t.Context(),
		[]string{`DELETE FROM user_roles WHERE user_id = ?<principal_id> AND role = ?<role>`},
		nil,
		nil,
		map[string]any{"principal_id": "user-1", "role": "admin"},
		true,
	)
	require.ErrorIs(t, err, ErrQueryAffectedZeroRows)
	require.False(t, deleted)
}

func TestRunRevokeProvisioning_ProbeErrorRollsBack(t *testing.T) {
	s, db := newRevokeProvisioningTestSyncer(t)
	seedUserWithRoles(t, db, "user-1", "admin")

	deleted, err := s.RunRevokeProvisioning(
		t.Context(),
		[]string{`DELETE FROM user_roles WHERE user_id = ?<principal_id> AND role = ?<role>`},
		nil,
		&PrincipalDeletedCheck{Query: `SELECT 1 FROM nonexistent_table WHERE id = ?<principal_id>`},
		map[string]any{"principal_id": "user-1", "role": "admin"},
		true,
	)
	require.Error(t, err)
	require.False(t, deleted)

	// the revoke DELETE was rolled back, so the role is still present
	require.Equal(t, 1, countRows(t, db, `SELECT COUNT(*) FROM user_roles WHERE user_id = ? AND role = ?`, "user-1", "admin"))
}

func withRevokeDeletesUserConfig(s *SQLSyncer) {
	s.config = ResourceType{
		StaticEntitlements: []*EntitlementMapping{
			{
				Id: "member",
				Provisioning: &EntitlementProvisioning{
					Vars: map[string]string{
						"principal_id": "principal.ID",
						"role":         "resource.ID",
					},
					Revoke: &EntitlementProvisioningQueries{
						Queries:               revokeDeletesUserQueries,
						PrincipalDeletedCheck: &PrincipalDeletedCheck{Query: `SELECT 1 FROM users WHERE id = ?<principal_id>`},
					},
				},
			},
		},
	}
}

func revokeGrantFor(userID, role string) *v2.Grant {
	roleResource := &v2.Resource{Id: &v2.ResourceId{ResourceType: "role", Resource: role}}
	principal := &v2.Resource{Id: &v2.ResourceId{ResourceType: "user", Resource: userID}}
	return sdkGrant.NewGrant(roleResource, "member", principal)
}

func TestRevoke_ReportsResourceDeletedOnLastRole(t *testing.T) {
	s, db := newRevokeProvisioningTestSyncer(t)
	withRevokeDeletesUserConfig(s)
	seedUserWithRoles(t, db, "user-1", "admin")

	annos, err := s.Revoke(t.Context(), revokeGrantFor("user-1", "admin"))
	require.NoError(t, err)

	resourceDeleted := &v2.ResourceDeleted{}
	ok, err := annos.Pick(resourceDeleted)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "user", resourceDeleted.GetResourceId().GetResourceType())
	require.Equal(t, "user-1", resourceDeleted.GetResourceId().GetResource())

	// not an "already revoked" case
	alreadyRevoked := &v2.GrantAlreadyRevoked{}
	ok, err = annos.Pick(alreadyRevoked)
	require.NoError(t, err)
	require.False(t, ok)

	require.Equal(t, 0, countRows(t, db, `SELECT COUNT(*) FROM users WHERE id = ?`, "user-1"))
}

func TestRevoke_ReportsResourceDeletedAndAlreadyRevokedOnRetry(t *testing.T) {
	s, _ := newRevokeProvisioningTestSyncer(t)
	withRevokeDeletesUserConfig(s)
	// nothing seeded: simulates a retry after the user was already deleted

	annos, err := s.Revoke(t.Context(), revokeGrantFor("user-1", "admin"))
	require.NoError(t, err)

	resourceDeleted := &v2.ResourceDeleted{}
	ok, err := annos.Pick(resourceDeleted)
	require.NoError(t, err)
	require.True(t, ok)

	alreadyRevoked := &v2.GrantAlreadyRevoked{}
	ok, err = annos.Pick(alreadyRevoked)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestRevoke_NoDeletedCheckReturnsNoResourceDeleted(t *testing.T) {
	s, db := newRevokeProvisioningTestSyncer(t)
	withRevokeDeletesUserConfig(s)
	// strip the deleted check to confirm the annotation is absent without it
	s.config.StaticEntitlements[0].Provisioning.Revoke.PrincipalDeletedCheck = nil
	seedUserWithRoles(t, db, "user-1", "admin")

	annos, err := s.Revoke(t.Context(), revokeGrantFor("user-1", "admin"))
	require.NoError(t, err)

	resourceDeleted := &v2.ResourceDeleted{}
	ok, err := annos.Pick(resourceDeleted)
	require.NoError(t, err)
	require.False(t, ok)
}
