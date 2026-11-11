//nolint:gosec // G101 false positives: string literals here are NHI trait config, not credentials.
package bsql

import (
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	sdkResource "github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/stretchr/testify/require"

	"github.com/conductorone/baton-sql/pkg/bcel"
	"google.golang.org/protobuf/proto"
)

// pickAnno reports whether the given annotation message is present on the resource.
func pickAnno(t *testing.T, r *v2.Resource, m proto.Message) bool {
	t.Helper()
	annos := annotations.Annotations(r.Annotations)
	ok, err := annos.Pick(m)
	require.NoError(t, err)
	return ok
}

// newTraitTestSyncer builds a SQLSyncer wired with a real CEL env and the given
// resource-type config, suitable for exercising the row -> resource mapping.
func newTraitTestSyncer(t *testing.T, rt ResourceType) *SQLSyncer {
	t.Helper()
	env, err := bcel.NewEnv(t.Context())
	require.NoError(t, err)
	return &SQLSyncer{
		resourceType: &v2.ResourceType{Id: "test"},
		config:       rt,
		env:          env,
	}
}

func baseMapping(traits *Traits, nhi *NonHumanIdentityMapping) ResourceType {
	return ResourceType{
		Name: "Test",
		List: &ListQuery{
			Query: "SELECT 1",
			Map: &ResourceMapping{
				Id:               ".id",
				DisplayName:      ".name",
				Traits:           traits,
				NonHumanIdentity: nhi,
			},
		},
	}
}

// K1 — a secret resource emits a SecretTrait with the mapped credential type/detail.
func TestMapResource_SecretTrait(t *testing.T) {
	ctx := t.Context()
	s := newTraitTestSyncer(t, baseMapping(&Traits{
		Secret: &SecretTraitMapping{
			CredentialType:   "'static_secret'",
			CredentialDetail: "'postgres.api_token'",
		},
	}, nil))

	r, err := s.mapResource(ctx, map[string]any{"id": "tok-1", "name": "API token"})
	require.NoError(t, err)

	st := &v2.SecretTrait{}
	require.True(t, pickAnno(t, r, st), "expected a SecretTrait annotation")
	require.Equal(t, v2.SecretTrait_CREDENTIAL_TYPE_STATIC_SECRET, st.GetCredentialType())
	require.Equal(t, "postgres.api_token", st.GetCredentialDetail())
}

func TestMapResource_SecretTrait_CredentialTypes(t *testing.T) {
	ctx := t.Context()
	cases := map[string]v2.SecretTrait_CredentialType{
		"'static_secret'":  v2.SecretTrait_CREDENTIAL_TYPE_STATIC_SECRET,
		"'asymmetric_key'": v2.SecretTrait_CREDENTIAL_TYPE_ASYMMETRIC_KEY,
		"'certificate'":    v2.SecretTrait_CREDENTIAL_TYPE_CERTIFICATE,
		"'bogus'":          v2.SecretTrait_CREDENTIAL_TYPE_UNSPECIFIED,
	}
	for expr, want := range cases {
		s := newTraitTestSyncer(t, baseMapping(&Traits{
			Secret: &SecretTraitMapping{CredentialType: expr},
		}, nil))
		r, err := s.mapResource(ctx, map[string]any{"id": "x", "name": "x"})
		require.NoError(t, err)
		st := &v2.SecretTrait{}
		require.True(t, pickAnno(t, r, st))
		require.Equal(t, want, st.GetCredentialType(), "expr %s", expr)
	}
}

// K2 — account_type mapping (already supported) emits SERVICE/SYSTEM.
func TestMapResource_AccountType(t *testing.T) {
	ctx := t.Context()
	cases := map[string]v2.UserTrait_AccountType{
		"'service'": v2.UserTrait_ACCOUNT_TYPE_SERVICE,
		"'system'":  v2.UserTrait_ACCOUNT_TYPE_SYSTEM,
		"'human'":   v2.UserTrait_ACCOUNT_TYPE_HUMAN,
	}
	for expr, want := range cases {
		s := newTraitTestSyncer(t, baseMapping(&Traits{
			User: &UserTraitMapping{AccountType: expr},
		}, nil))
		r, err := s.mapResource(ctx, map[string]any{"id": "svc", "name": "svc"})
		require.NoError(t, err)
		ut, err := sdkResource.GetUserTrait(r)
		require.NoError(t, err)
		require.Equal(t, want, ut.GetAccountType(), "expr %s", expr)
	}
}

// K3 — NHI is kind-agnostic: it co-exists with a primary (app) trait.
func TestMapResource_NonHumanIdentity_WithApp(t *testing.T) {
	ctx := t.Context()
	s := newTraitTestSyncer(t, baseMapping(
		&Traits{App: &AppTraitMapping{}},
		&NonHumanIdentityMapping{
			NhiType:   "'assumable_role'",
			NhiDetail: "'aws.iam_role'",
		},
	))

	r, err := s.mapResource(ctx, map[string]any{"id": "role-1", "name": "deploy-role"})
	require.NoError(t, err)

	// primary app trait still present
	_, err = sdkResource.GetAppTrait(r)
	require.NoError(t, err, "expected the primary AppTrait to remain")

	nhi, err := sdkResource.GetNonHumanIdentityTrait(r)
	require.NoError(t, err)
	require.Equal(t, v2.NonHumanIdentityTrait_NHI_TYPE_ASSUMABLE_ROLE, nhi.GetNhiType())
	require.Equal(t, "aws.iam_role", nhi.GetNhiDetail())
}

// K3 — NHI may stand alone with no primary trait.
func TestMapResource_NonHumanIdentity_Standalone(t *testing.T) {
	ctx := t.Context()
	s := newTraitTestSyncer(t, baseMapping(nil, &NonHumanIdentityMapping{
		NhiType:   "'managed_identity'",
		NhiDetail: "'azure.managed_identity'",
	}))

	r, err := s.mapResource(ctx, map[string]any{"id": "mi-1", "name": "vm-identity"})
	require.NoError(t, err)

	nhi, err := sdkResource.GetNonHumanIdentityTrait(r)
	require.NoError(t, err)
	require.Equal(t, v2.NonHumanIdentityTrait_NHI_TYPE_MANAGED_IDENTITY, nhi.GetNhiType())
}

// Agent — emits an AgentTrait with status, identity ref, and profile.
func TestMapResource_AgentTrait(t *testing.T) {
	ctx := t.Context()
	s := newTraitTestSyncer(t, baseMapping(&Traits{
		Agent: &AgentTraitMapping{
			Status:               "'ready'",
			IdentityResourceType: "'user'",
			IdentityResourceID:   ".identity_id",
			Profile:              map[string]string{"model": ".model"},
		},
	}, nil))

	r, err := s.mapResource(ctx, map[string]any{
		"id":          "agent-1",
		"name":        "support-bot",
		"identity_id": "svc-acct-1",
		"model":       "claude-opus",
	})
	require.NoError(t, err)

	at, err := sdkResource.GetAgentTrait(r)
	require.NoError(t, err)
	// Status and profile live on Resource (trait-level getters are deprecated SA1019).
	// Agent READY maps to RESOURCE_STATUS_ENABLED (identical enum values).
	st := sdkResource.GetStatus(r)
	require.NotNil(t, st)
	require.Equal(t, v2.Status_RESOURCE_STATUS_ENABLED, st.GetStatus())
	require.NotNil(t, at.GetIdentityResourceId())
	require.Equal(t, "user", at.GetIdentityResourceId().GetResourceType())
	require.Equal(t, "svc-acct-1", at.GetIdentityResourceId().GetResource())
	profile := sdkResource.GetProfile(r)
	require.NotNil(t, profile)
	require.Equal(t, "claude-opus", profile.GetFields()["model"].GetStringValue())
}

// Graceful degradation — a plain user resource emits only a UserTrait, no
// secret/NHI/agent annotations.
func TestMapResource_NoNHIByDefault(t *testing.T) {
	ctx := t.Context()
	s := newTraitTestSyncer(t, baseMapping(&Traits{
		User: &UserTraitMapping{},
	}, nil))

	r, err := s.mapResource(ctx, map[string]any{"id": "u1", "name": "Jane"})
	require.NoError(t, err)

	_, err = sdkResource.GetUserTrait(r)
	require.NoError(t, err)

	require.False(t, pickAnno(t, r, &v2.SecretTrait{}), "no SecretTrait expected")
	require.False(t, pickAnno(t, r, &v2.NonHumanIdentityTrait{}), "no NonHumanIdentityTrait expected")
	require.False(t, pickAnno(t, r, &v2.AgentTrait{}), "no AgentTrait expected")
}

// The shipped nhi-example.yml parses and advertises the expected traits.
func TestNHIExampleConfig(t *testing.T) {
	ctx := t.Context()
	raw := loadExampleConfig(t, "nhi-example")
	c, err := Parse([]byte(raw))
	require.NoError(t, err)

	want := map[string][]v2.ResourceType_Trait{
		"service_account": {v2.ResourceType_TRAIT_USER},
		"api_token":       {v2.ResourceType_TRAIT_SECRET},
		"iam_role":        {v2.ResourceType_TRAIT_APP},
		"ai_agent":        {v2.ResourceType_TRAIT_AGENT},
	}
	for rtID, wantTraits := range want {
		got, err := c.extractTraits(rtID)
		require.NoError(t, err, rtID)
		require.Equal(t, wantTraits, got, rtID)
	}

	// iam_role declares a kind-agnostic NHI mapping alongside its app trait.
	require.NotNil(t, c.ResourceTypes["iam_role"].List.Map.NonHumanIdentity)

	rts, err := c.GetResourceTypes(ctx)
	require.NoError(t, err)
	require.Len(t, rts, 4)
}

// Resource-type advertisement includes TRAIT_SECRET / TRAIT_AGENT.
func TestExtractTraits_SecretAndAgent(t *testing.T) {
	ctx := t.Context()
	_ = ctx
	cfg := Config{ResourceTypes: map[string]ResourceType{
		"secret": baseMapping(&Traits{Secret: &SecretTraitMapping{CredentialType: "'static_secret'"}}, nil),
		"agent":  baseMapping(&Traits{Agent: &AgentTraitMapping{Status: "'ready'"}}, nil),
	}}

	secretTraits, err := cfg.extractTraits("secret")
	require.NoError(t, err)
	require.Equal(t, []v2.ResourceType_Trait{v2.ResourceType_TRAIT_SECRET}, secretTraits)

	agentTraits, err := cfg.extractTraits("agent")
	require.NoError(t, err)
	require.Equal(t, []v2.ResourceType_Trait{v2.ResourceType_TRAIT_AGENT}, agentTraits)
}
