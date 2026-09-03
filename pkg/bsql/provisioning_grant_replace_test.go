package bsql

import (
	"database/sql"
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sql/pkg/bcel"
	"github.com/conductorone/baton-sql/pkg/database"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// withGrantReplaceConfig wires a "member" entitlement whose grant replaces the
// principal's existing role: the grant_replace query finds the old membership and
// revokes it, then the main grant runs. The main grant uses INSERT OR IGNORE so a
// pre-existing target row makes it affect zero rows (the already-granted path).
func withGrantReplaceConfig(s *SQLSyncer, noTransaction bool) {
	s.resourceType = &v2.ResourceType{Id: "role"}
	s.config = ResourceType{
		StaticEntitlements: []*EntitlementMapping{
			{
				Id: "member",
				Provisioning: &EntitlementProvisioning{
					Vars: map[string]string{
						"user_id": "principal.ID",
						"role":    "resource.ID",
					},
					Grant: &GrantEntitlementProvisioningQueries{
						EntitlementProvisioningQueries: EntitlementProvisioningQueries{
							NoTransaction: noTransaction,
							Queries:       []string{`INSERT OR IGNORE INTO user_roles (user_id, role) VALUES (?<user_id>, ?<role>)`},
						},
						GrantReplace: &GrantReplaceProvisioningQueries{
							Query: `SELECT user_id, role FROM user_roles WHERE user_id = ?<user_id> AND role = 'viewer'`,
							Map: []*GrantMapping{
								{
									EntitlementResourceId: ".role",
									PrincipalId:           ".user_id",
									PrincipalType:         "user",
									Entitlement:           "member",
								},
							},
						},
					},
					Revoke: &RevokeEntitlementProvisioningQueries{
						EntitlementProvisioningQueries: EntitlementProvisioningQueries{
							Queries: []string{`DELETE FROM user_roles WHERE user_id = ?<user_id> AND role = ?<role>`},
						},
					},
				},
			},
		},
	}
}

func newGrantReplaceTestSyncer(t *testing.T) (*SQLSyncer, *sql.DB) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	_, err = db.ExecContext(t.Context(), `CREATE TABLE user_roles (user_id TEXT, role TEXT, UNIQUE(user_id, role))`)
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

// Transactional path: the target grant already exists, so the main grant hits the
// zero-rows sentinel and the tx rolls back, undoing the grant_replace revoke. The
// response must NOT claim GrantReplaced, and the old row must survive.
func TestGrant_ReplaceRolledBackDoesNotReportGrantReplaced(t *testing.T) {
	s, db := newGrantReplaceTestSyncer(t)
	withGrantReplaceConfig(s, false) // transactional
	_, err := db.ExecContext(t.Context(), `INSERT INTO user_roles (user_id, role) VALUES ('user-1','viewer'), ('user-1','admin')`)
	require.NoError(t, err)

	annos, err := s.Grant(t.Context(), userPrincipal("user-1"), memberEntitlementFor("admin"))
	require.NoError(t, err)

	exists, err := annos.Pick(&v2.GrantAlreadyExists{})
	require.NoError(t, err)
	require.True(t, exists)

	replaced, err := annos.Pick(&v2.GrantReplaced{})
	require.NoError(t, err)
	require.False(t, replaced, "GrantReplaced must not be reported when the tx rolled back")

	// the replace revoke was rolled back, so the old membership survives
	require.Equal(t, 1, countRows(t, db, `SELECT COUNT(*) FROM user_roles WHERE user_id = ? AND role = ?`, "user-1", "viewer"))
}

// no_transaction path: the grant_replace revoke commits immediately, so even when the
// main grant hits the zero-rows sentinel the removal really happened and GrantReplaced
// must be reported.
func TestGrant_ReplaceCommittedReportsGrantReplaced(t *testing.T) {
	s, db := newGrantReplaceTestSyncer(t)
	withGrantReplaceConfig(s, true) // no_transaction
	_, err := db.ExecContext(t.Context(), `INSERT INTO user_roles (user_id, role) VALUES ('user-1','viewer'), ('user-1','admin')`)
	require.NoError(t, err)

	annos, err := s.Grant(t.Context(), userPrincipal("user-1"), memberEntitlementFor("admin"))
	require.NoError(t, err)

	exists, err := annos.Pick(&v2.GrantAlreadyExists{})
	require.NoError(t, err)
	require.True(t, exists)

	replaced, err := annos.Pick(&v2.GrantReplaced{})
	require.NoError(t, err)
	require.True(t, replaced, "GrantReplaced must be reported when the replace committed")

	// the replace revoke committed, so the old membership is gone
	require.Equal(t, 0, countRows(t, db, `SELECT COUNT(*) FROM user_roles WHERE user_id = ? AND role = ?`, "user-1", "viewer"))
}
