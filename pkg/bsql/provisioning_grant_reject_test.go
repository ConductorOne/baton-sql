package bsql

import (
	"database/sql"
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	sdkEntitlement "github.com/conductorone/baton-sdk/pkg/types/entitlement"
	"github.com/conductorone/baton-sql/pkg/bcel"
	"github.com/conductorone/baton-sql/pkg/database"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func newGrantProvisioningTestSyncer(t *testing.T) (*SQLSyncer, *sql.DB) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	_, err = db.Exec(`CREATE TABLE user_roles (user_id TEXT PRIMARY KEY, role TEXT)`)
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

func TestRunGrantProvisioning_RejectIfMatchReturnsGrantRejectedAndSkipsMutations(t *testing.T) {
	s, db := newGrantProvisioningTestSyncer(t)

	annos, err := s.RunGrantProvisioning(
		t.Context(),
		nil,
		[]string{`INSERT INTO user_roles (user_id, role) VALUES (?<user_id>, ?<role>)`},
		nil,
		map[string]any{
			"user_id": "user-1",
			"role":    "admin",
		},
		true,
		nil,
		&GrantRejectIfProvisioningQuery{
			Query:  `SELECT 1 AS rejected`,
			Reason: `'Grant rejected: user already has a mutually exclusive role.'`,
		},
	)
	require.NoError(t, err)

	rejected := &v2.GrantRejected{}
	ok, err := annos.Pick(rejected)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "Grant rejected: user already has a mutually exclusive role.", rejected.GetReason())

	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM user_roles`).Scan(&count))
	require.Equal(t, 0, count)
}

func TestRunGrantProvisioning_RejectIfNoMatchProceedsWithGrant(t *testing.T) {
	s, db := newGrantProvisioningTestSyncer(t)

	annos, err := s.RunGrantProvisioning(
		t.Context(),
		nil,
		[]string{`INSERT INTO user_roles (user_id, role) VALUES (?<user_id>, ?<role>)`},
		nil,
		map[string]any{
			"user_id": "user-1",
			"role":    "admin",
		},
		true,
		nil,
		&GrantRejectIfProvisioningQuery{
			Query:  `SELECT 1 AS rejected WHERE 0`,
			Reason: `'Grant rejected: user already has a mutually exclusive role.'`,
		},
	)
	require.NoError(t, err)

	rejected := &v2.GrantRejected{}
	ok, err := annos.Pick(rejected)
	require.NoError(t, err)
	require.False(t, ok)

	var role string
	require.NoError(t, db.QueryRow(`SELECT role FROM user_roles WHERE user_id = ?`, "user-1").Scan(&role))
	require.Equal(t, "admin", role)
}

func TestGrant_ZeroRowWithoutRejectIfStillReturnsGrantAlreadyExists(t *testing.T) {
	s, _ := newGrantProvisioningTestSyncer(t)

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
							Queries: []string{`UPDATE user_roles SET role = ?<role> WHERE user_id = ?<user_id>`},
						},
					},
				},
			},
		},
	}

	roleResource := &v2.Resource{
		Id: &v2.ResourceId{
			ResourceType: "role",
			Resource:     "admin",
		},
	}
	principal := &v2.Resource{
		Id: &v2.ResourceId{
			ResourceType: "user",
			Resource:     "user-1",
		},
	}
	entitlement := sdkEntitlement.NewAssignmentEntitlement(roleResource, "member")

	annos, err := s.Grant(t.Context(), principal, entitlement)
	require.NoError(t, err)

	alreadyExists := &v2.GrantAlreadyExists{}
	ok, err := annos.Pick(alreadyExists)
	require.NoError(t, err)
	require.True(t, ok)
}
