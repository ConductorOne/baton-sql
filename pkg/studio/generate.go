package studio

import (
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
	hasManager := false
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
			hasManager = true
		default:
			if key, ok := profileKey(fm.Field); ok { // "profile.department" -> "department"
				putScalar(profile, key, cel)
			}
		}
	}
	// Trap #2: manager fields require a non-empty profile.
	if hasManager && len(profile.Content) == 0 {
		// Surface manager_id into profile so it is not silently dropped.
		putScalar(profile, "manager_id", ".manager_id")
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

	// Trap #3: no entitlements and no grants => skip flag, emitted as a real bool.
	if rt.Entitlements.Mode == "none" && len(rt.Grants) == 0 {
		putBool(n, "skip_entitlements_and_grants", true)
	}
	return n, nil
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
