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
		node, err := genResourceType(spec, rt)
		if err != nil {
			return nil, err
		}
		putNode(rts, rt.ID, node)
	}
	putNode(root, "resource_types", rts)
	return yaml.Marshal(root)
}

func genResourceType(spec *Spec, rt *ResourceTypeSpec) (*yaml.Node, error) {
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

	switch rt.Entitlements.Mode {
	case "static":
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, e := range rt.Entitlements.Static {
			item := mapNode()
			putScalar(item, "id", e.ID)
			putScalar(item, "display_name", e.DisplayName)
			if e.Purpose != "" {
				putScalar(item, "purpose", e.Purpose)
			}
			seq.Content = append(seq.Content, item)
		}
		putNode(n, "static_entitlements", seq)
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
			case "grantable_to":
				putSeq(mp, "grantable_to", cel)
			}
			switch fm.Field {
			case "display_name":
				displayCELForSlug = cel
			case "id":
				idCELForSlug = cel
			}
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
	}

	// Trap #3: no entitlements and no grants => skip flag, emitted as a real bool.
	if rt.Entitlements.Mode == "none" && len(rt.Grants) == 0 {
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

func profileKey(field string) (string, bool) {
	const p = "profile."
	if len(field) > len(p) && field[:len(p)] == p {
		return field[len(p):], true
	}
	return "", false
}
