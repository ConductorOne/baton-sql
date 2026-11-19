//nolint:gosec // These credentials are isolated test sentinels, not real secrets.
package bsql

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sql/pkg/database"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/types/known/structpb"
)

func captureQueryLogs(t *testing.T) (context.Context, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&logs), zap.DebugLevel)
	return ctxzap.ToContext(t.Context(), zap.New(core)), &logs
}

func assertQueryLogsExclude(t *testing.T, logs *bytes.Buffer, secrets ...string) {
	t.Helper()
	require.NotEmpty(t, logs.String(), "the real execution path must emit diagnostics")
	for _, secret := range secrets {
		assert.NotContains(t, logs.String(), secret)
	}
	decoder := json.NewDecoder(bytes.NewReader(logs.Bytes()))
	for decoder.More() {
		var fields map[string]any
		require.NoError(t, decoder.Decode(&fields))
		// Also inspect decoded values, so JSON escaping cannot hide a leak.
		for _, secret := range secrets {
			assertLogValueExcludes(t, fields, secret)
		}
	}
}

func assertLogValueExcludes(t *testing.T, value any, secret string) {
	t.Helper()
	switch value := value.(type) {
	case string:
		assert.NotContains(t, value, secret)
	case []any:
		for _, element := range value {
			assertLogValueExcludes(t, element, secret)
		}
	case map[string]any:
		for key, element := range value {
			assert.NotContains(t, key, secret)
			assertLogValueExcludes(t, element, secret)
		}
	}
}

func TestQueryLoggingDoesNotExposeValues(t *testing.T) {
	const boundSecret = "BoundCredentialSentinel\"42"
	const inlineSecret = "InlineCredentialSentinel42"
	const unusedSecret = "UnusedCredentialSentinel42"
	for _, path := range []string{
		"exec", "grant_exec", "validation", "validation_empty", "validation_empty_ddl",
		"exists_check", "row_query", "row_query_error", "inline_exec", "inline_query", "grant_replace",
	} {
		t.Run(path, func(t *testing.T) {
			s, db := newRevokeProvisioningTestSyncer(t)
			ctx, logs := captureQueryLogs(t)
			vars := map[string]any{
				"password":    boundSecret,
				"inline":      inlineSecret,
				"credentials": map[string]any{"password": unusedSecret},
			}
			query := `INSERT INTO user_roles (user_id, role) VALUES (?<password>, 'viewer')`
			switch path {
			case "exec", "inline_exec":
				if path == "inline_exec" {
					query += ` /* ` + inlineSecret + ` */`
				}
				require.NoError(t, s.RunProvisioningQueries(ctx, []string{query}, nil, vars, ProvisioningOptions{UseTransaction: true}))
				require.Equal(t, 1, countRows(t, db, `SELECT COUNT(*) FROM user_roles WHERE user_id = ?`, boundSecret))
			case "grant_exec":
				_, err := s.RunGrantProvisioning(ctx, nil, []string{query}, nil, vars, ProvisioningOptions{UseTransaction: true}, nil, nil)
				require.NoError(t, err)
				require.Equal(t, 1, countRows(t, db, `SELECT COUNT(*) FROM user_roles WHERE user_id = ?`, boundSecret))
			case "grant_replace":
				replace := &GrantReplaceProvisioningQueries{Query: `SELECT 1 /* ` + inlineSecret + ` */ WHERE 1 = 0`}
				_, err := s.RunGrantProvisioning(ctx, nil, []string{query}, nil, vars, ProvisioningOptions{UseTransaction: true}, replace, nil)
				require.NoError(t, err)
				require.Equal(t, 1, countRows(t, db, `SELECT COUNT(*) FROM user_roles WHERE user_id = ?`, boundSecret))
			case "validation":
				validation := []string{`SELECT 1 WHERE ?<password> = ?<password>`}
				require.NoError(t, s.RunProvisioningQueriesWithExecutor(ctx, []string{query}, validation, "test", vars, db, ProvisioningOptions{}))
				require.Equal(t, 1, countRows(t, db, `SELECT COUNT(*) FROM user_roles WHERE user_id = ?`, boundSecret))
			case "validation_empty", "validation_empty_ddl":
				if path == "validation_empty_ddl" {
					s.dbEngine = database.DB2
				}
				validation := []string{`SELECT ?<password> AS ?<inline|identifier> WHERE 1 = 0`}
				err := s.RunProvisioningQueriesWithExecutor(ctx, []string{query}, validation, "test", vars, db, ProvisioningOptions{})
				require.Error(t, err)
				if path == "validation_empty_ddl" {
					require.ErrorIs(t, err, ErrValidationNoRows)
				}
				assert.NotContains(t, err.Error(), inlineSecret, "returned errors can also be logged by callers")
				require.Equal(t, 0, countRows(t, db, `SELECT COUNT(*) FROM user_roles`))
			case "exists_check":
				_, err := db.ExecContext(ctx, `INSERT INTO users (id) VALUES (?)`, boundSecret)
				require.NoError(t, err)
				exists, err := s.runPrincipalExistsCheck(ctx, db, &PrincipalExistsCheck{Query: `SELECT 1 FROM users WHERE id = ?<password>`}, vars)
				require.NoError(t, err)
				require.True(t, exists)
			case "row_query", "inline_query":
				query = `SELECT ?<password> AS value`
				column := "value"
				if path == "inline_query" {
					query = `SELECT ?<password> AS ?<inline|identifier>`
					column = inlineSecret
				}
				var rows int
				_, err := s.runQuery(ctx, db, nil, query, nil, vars, func(_ context.Context, row map[string]any) (bool, error) {
					rows++
					require.Equal(t, boundSecret, row[column])
					return true, nil
				})
				require.NoError(t, err)
				require.Equal(t, 1, rows)
			case "row_query_error":
				_, err := s.runQuery(ctx, db, nil, `SELECT ?<password> FROM absent_table`, nil, vars, func(context.Context, map[string]any) (bool, error) {
					t.Fatal("failed queries must not invoke the row callback")
					return false, nil
				})
				require.Error(t, err)
			}
			assertQueryLogsExclude(t, logs, boundSecret, inlineSecret, unusedSecret)
		})
	}
}

func TestAccountCredentialsStayOutOfQueryLogs(t *testing.T) {
	s, db := newRevokeProvisioningTestSyncer(t)
	_, err := db.ExecContext(t.Context(), `CREATE TABLE credentials (username TEXT PRIMARY KEY, password TEXT)`)
	require.NoError(t, err)
	s.resourceType = &v2.ResourceType{Id: "user"}
	s.config = ResourceType{
		List: &ListQuery{Map: &ResourceMapping{Id: "cols.username", DisplayName: "cols.username"}},
		AccountProvisioning: &AccountProvisioning{
			Schema: []*AccountProvisioningField{{Name: "username", Type: "string"}},
			Create: &AccountCreationConfig{Queries: []string{`INSERT INTO credentials (username, password) VALUES (?<username>, ?<password>)`}},
			Validate: &AccountValidationConfig{
				Vars:  map[string]string{"username": "username", "password": "password"},
				Query: `SELECT username FROM credentials WHERE username = ?<username> AND password = ?<password>`,
			},
		},
		CredentialRotation: &CredentialRotation{
			Update: &AccountCreationConfig{Queries: []string{`UPDATE credentials SET password = ?<password> WHERE username = ?<resource_id>`}},
		},
	}
	s.fullConfig = Config{ResourceTypes: map[string]ResourceType{"user": s.config}}
	user := &userSyncer{SQLSyncer: s}
	options := &v2.LocalCredentialOptions{Options: &v2.LocalCredentialOptions_RandomPassword_{RandomPassword: &v2.LocalCredentialOptions_RandomPassword{Length: 32}}}
	ctx, logs := captureQueryLogs(t)
	_, created, _, err := user.CreateAccount(ctx, &v2.AccountInfo{Profile: &structpb.Struct{Fields: map[string]*structpb.Value{"username": structpb.NewStringValue("fixture-user")}}}, options)
	require.NoError(t, err)
	require.Len(t, created, 1)
	var stored string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT password FROM credentials WHERE username = 'fixture-user'`).Scan(&stored))
	require.Equal(t, string(created[0].Bytes), stored)
	assertQueryLogsExclude(t, logs, stored)

	logs.Reset()
	rotated, _, err := user.Rotate(ctx, &v2.ResourceId{ResourceType: "user", Resource: "fixture-user"}, options)
	require.NoError(t, err)
	require.Len(t, rotated, 1)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT password FROM credentials WHERE username = 'fixture-user'`).Scan(&stored))
	require.Equal(t, string(rotated[0].Bytes), stored)
	assertQueryLogsExclude(t, logs, stored)
}
