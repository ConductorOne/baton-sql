package studio

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Generate renders a Spec into baton-sql connector YAML. It builds the
// output as a yaml.Node mapping so only keys that are actually set get
// serialized — this is what lets us avoid emitting no-op trait keys
// (e.g. mfa_enabled/sso_enabled) that would otherwise show up as empty
// strings from a fully-populated bsql.Config struct marshal.
func Generate(spec *Spec) ([]byte, error) {
	root := mapNode()
	putScalar(root, "app_name", spec.AppName)

	rts := mapNode()
	for i := range spec.ResourceTypes {
		rt := &spec.ResourceTypes[i]
		node, err := genResourceType(rt)
		if err != nil {
			return nil, err
		}
		putNode(rts, rt.ID, node)
	}
	putNode(root, "resource_types", rts)
	return yaml.Marshal(root)
}

func genResourceType(rt *ResourceTypeSpec) (*yaml.Node, error) {
	n := mapNode()
	putScalar(n, "name", rt.Name)

	list := mapNode()
	putScalar(list, "query", rt.List.Query)
	mp := mapNode()
	traits := mapNode() // trait fields collected here for user/group/role/app
	profile := mapNode()
	managerFields := map[string]string{} // field name ("manager_id"/"manager_email") -> its computed CEL
	for _, fm := range rt.List.Fields {
		cel, err := CompileField(fm)
		if err != nil {
			return nil, err
		}
		switch fm.Field {
		case "id", "display_name", "description":
			putScalar(mp, fm.Field, cel)
		case "emails":
			putSeq(traits, "emails", cel)
		case "employee_ids":
			putSeq(traits, "employee_ids", cel)
		case "login_aliases":
			putSeq(traits, "login_aliases", cel)
		case "status", "account_type", "login", "last_login", "status_details":
			putScalar(traits, fm.Field, cel)
		case "manager_id", "manager_email":
			putScalar(traits, fm.Field, cel)
			managerFields[fm.Field] = cel
		default:
			if key, ok := profileKey(fm.Field); ok { // "profile.department" -> "department"
				putScalar(profile, key, cel)
			}
		}
	}
	// Trap #2: manager fields require a non-empty profile. Use the actual computed
	// CEL (and actual field name) for whichever manager field(s) were mapped —
	// never a hardcoded ".manager_id", which would be wrong whenever the source
	// column isn't literally named "manager_id" or only manager_email is mapped.
	if len(managerFields) > 0 && len(profile.Content) == 0 {
		for _, name := range []string{"manager_id", "manager_email"} {
			if cel, ok := managerFields[name]; ok {
				putScalar(profile, name, cel)
			}
		}
	}
	if len(profile.Content) > 0 {
		putNode(traits, "profile", profile)
	}
	if len(traits.Content) > 0 && rt.Trait != "" && rt.Trait != "none" {
		wrap := mapNode()
		putNode(wrap, rt.Trait, traits) // Trap #1: mfa/sso never added here
		putNode(mp, "traits", wrap)
	}
	putNode(list, "map", mp)
	putNode(n, "list", list)

	// Track whether an entitlements block was actually emitted so the skip
	// flag can key on real emptiness rather than the Mode sentinel (Trap #3):
	// a "static" mode with an empty list, for example, emits nothing.
	entitlementsEmitted := false
	mode := rt.Entitlements.Mode
	switch mode {
	case "", "none":
		// no entitlements block
	case "static":
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, e := range rt.Entitlements.Static {
			item := mapNode()
			putScalar(item, "id", e.ID)
			putScalar(item, "display_name", e.DisplayName)
			if e.Description != "" {
				putScalar(item, "description", e.Description)
			}
			if e.Purpose != "" {
				putScalar(item, "purpose", e.Purpose)
			}
			// grantable_to is a literal list of resource-type IDs, not a
			// per-row CEL ref (which would silently match nothing).
			if len(e.GrantableTo) > 0 {
				putLiteralSeq(item, "grantable_to", e.GrantableTo)
			}
			if e.Immutable {
				putBool(item, "immutable", true)
			}
			seq.Content = append(seq.Content, item)
		}
		if len(seq.Content) > 0 {
			putNode(n, "static_entitlements", seq)
			entitlementsEmitted = true
		}
	case "query":
		ent := mapNode()
		putScalar(ent, "query", rt.Entitlements.Query)
		mp := mapNode()
		var displayCELForSlug, idCELForSlug string
		for _, fm := range rt.Entitlements.Fields {
			cel, err := CompileField(fm)
			if err != nil {
				return nil, err
			}
			switch fm.Field {
			case "id", "display_name", "description", "purpose", "slug":
				putScalar(mp, fm.Field, cel)
			}
			switch fm.Field {
			case "display_name":
				displayCELForSlug = cel
			case "id":
				idCELForSlug = cel
			}
		}
		// grantable_to is a literal list of resource-type IDs sourced from the
		// spec (EntitlementsSpec.GrantableTo), NOT a CEL ref compiled from a
		// query column — the bsql grantable_to matcher compares each entry
		// literally against defined resource-type IDs, so a CEL ref like
		// ".column" would silently match nothing.
		if len(rt.Entitlements.GrantableTo) > 0 {
			putLiteralSeq(mp, "grantable_to", rt.Entitlements.GrantableTo)
		}
		// Trap #3: always emit a valid slug. Prefer display_name, then fall
		// back to id — never emit slugify() with an empty argument, which
		// would silently break the "always emit a VALID slug" guarantee
		// (bsql.EntitlementMapping.Slug is an unvalidated string, so a bad
		// slug wouldn't be caught by bsql.Parse).
		if !hasKey(mp, "slug") {
			slugSrc := displayCELForSlug
			if slugSrc == "" {
				slugSrc = idCELForSlug
			}
			if slugSrc == "" {
				return nil, fmt.Errorf("resource type %q: dynamic entitlements need a display_name or id mapping to derive a slug", rt.ID)
			}
			putScalar(mp, "slug", fmt.Sprintf("%s(%s)", RecipeSlugify, slugSrc))
		}
		// bsql.EntitlementsQuery.Map is []*EntitlementMapping (a YAML sequence),
		// not a single mapping like ListQuery.Map — wrap our one built mapping
		// in a sequence so this round-trips through bsql.Parse. Emitting `map:`
		// as a bare mapping node fails yaml.v3 unmarshal into the slice field.
		mapSeq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		mapSeq.Content = append(mapSeq.Content, mp)
		putNode(ent, "map", mapSeq)
		putNode(n, "entitlements", ent)
		entitlementsEmitted = true
	default:
		// An unrecognized mode would otherwise be silently treated as "none"
		// (emitting a skip flag) — surface it as an error instead.
		return nil, fmt.Errorf("resource type %q: unknown entitlements mode %q (want static, query, none, or empty)", rt.ID, mode)
	}

	grantsEmitted := false
	if len(rt.Grants) > 0 {
		gseq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, g := range rt.Grants {
			gm := mapNode()
			putScalar(gm, "query", g.Query)
			if g.ResourceVar != "" {
				vars := mapNode()
				putScalar(vars, g.ResourceVar, "resource.ID")
				putNode(gm, "vars", vars)
			}
			// One bsql grant map: entry per GrantMapping. Key order per row is
			// principal_id, skip_if (when set), principal_type, entitlement_id
			// (when set) — held fixed so Studio-generated configs round-trip
			// byte-identically.
			mseq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			for _, m := range g.Mappings {
				row := mapNode()
				pidCEL, err := CompileField(m.PrincipalID)
				if err != nil {
					return nil, err
				}
				putScalar(row, "principal_id", pidCEL)
				if m.SkipIf != nil {
					skipCEL, err := CompileField(*m.SkipIf)
					if err != nil {
						return nil, err
					}
					putScalar(row, "skip_if", skipCEL)
				}
				putScalar(row, "principal_type", m.PrincipalType)
				if m.Entitlement != "" {
					putScalar(row, "entitlement_id", m.Entitlement)
				}
				mseq.Content = append(mseq.Content, row)
			}
			putNode(gm, "map", mseq)
			gseq.Content = append(gseq.Content, gm)
		}
		putNode(n, "grants", gseq)
		grantsEmitted = true
	}

	// Trap #3: nothing actually emitted for entitlements AND grants => skip
	// flag, emitted as a real bool. Keyed on real emptiness, not the Mode
	// sentinel, so an empty static list (which emits nothing) still gets the
	// flag.
	if !entitlementsEmitted && !grantsEmitted {
		putBool(n, "skip_entitlements_and_grants", true)
	}
	return n, nil
}

func hasKey(m *yaml.Node, key string) bool {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return true
		}
	}
	return false
}

func mapNode() *yaml.Node { return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"} }

func putScalar(m *yaml.Node, key, val string) {
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Value: val})
}

func putBool(m *yaml.Node, key string, val bool) {
	v := "false"
	if val {
		v = "true"
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: v})
}

func putNode(m *yaml.Node, key string, v *yaml.Node) {
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, v)
}

func putSeq(m *yaml.Node, key, item string) {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: item})
	putNode(m, key, seq)
}

// putLiteralSeq emits a YAML sequence of literal string scalars (not CEL
// refs). Used for grantable_to, whose entries bsql compares literally against
// defined resource-type IDs.
func putLiteralSeq(m *yaml.Node, key string, items []string) {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, it := range items {
		seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: it})
	}
	putNode(m, key, seq)
}

func profileKey(field string) (string, bool) {
	const p = "profile."
	if len(field) > len(p) && field[:len(p)] == p {
		return field[len(p):], true
	}
	return "", false
}
