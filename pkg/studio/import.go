package studio

import (
	"fmt"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/conductorone/baton-sql/pkg/bsql"
)

// columnRefRE matches a "clean" column pick CEL expression, i.e. exactly what
// colRef (recipes.go) produces for a plain column reference: ".<column>".
// Anything that doesn't match this shape is treated as richer CEL that isn't
// safe to reverse into a recipe, so it comes back as an editable raw-CEL
// mapping instead (see celToMapping).
var columnRefRE = regexp.MustCompile(`^\.([A-Za-z0-9_]+)$`)

// traitScalarOrder is the fixed order in which trait-scalar canonical fields
// are reconstructed from a parsed UserTraitMapping. The exact order doesn't
// affect the regenerated YAML (each field lands in its own map/seq entry,
// and profile is always emitted last by genResourceType regardless of
// interleaving), but a fixed order keeps SpecFromConfig's output
// deterministic across runs.
var traitScalarOrder = []string{
	"emails", "status", "account_type", "login", "login_aliases",
	"last_login", "employee_ids", "manager_id", "manager_email", "status_details",
}

// SpecFromYAML parses baton-sql connector YAML and reverses it into a Studio
// Spec: bsql.Parse(data), then SpecFromConfig(cfg). bsql.Config.ResourceTypes
// is a Go map, so that trip through bsql.Parse loses the original top-level
// resource_types key order; SpecFromConfig alone can only pick something
// deterministic (sorted). To keep Generate(SpecFromYAML(Generate(spec)))
// byte-identical to Generate(spec) - the property that matters for a Studio
// "load YAML" workflow, since Generate emits resource types in Spec slice
// order - SpecFromYAML additionally recovers the real key order with a
// second, narrow yaml.Node parse of the same bytes and reorders the result to
// match. This doesn't change what SpecFromConfig itself returns for callers
// that already have a *bsql.Config with no source text to consult.
func SpecFromYAML(data []byte) (*Spec, error) {
	cfg, err := bsql.Parse(data)
	if err != nil {
		return nil, err
	}
	spec, err := SpecFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	reorderResourceTypes(spec, resourceTypeOrderFromYAML(data))
	return spec, nil
}

// resourceTypeOrderFromYAML extracts the top-level resource_types mapping's
// key order directly from the source YAML via yaml.Node, sidestepping the
// order loss that comes from decoding into bsql.Config's
// map[string]ResourceType. Returns nil (silently) on any shape it doesn't
// recognize - resourceTypeOrderFromYAML is a best-effort ordering hint, not a
// second parser; bsql.Parse (already called by the time this runs) is the
// authority on whether data is valid.
func resourceTypeOrderFromYAML(data []byte) []string {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "resource_types" {
			continue
		}
		rtNode := root.Content[i+1]
		if rtNode.Kind != yaml.MappingNode {
			return nil
		}
		order := make([]string, 0, len(rtNode.Content)/2)
		for j := 0; j+1 < len(rtNode.Content); j += 2 {
			order = append(order, rtNode.Content[j].Value)
		}
		return order
	}
	return nil
}

// reorderResourceTypes rearranges spec.ResourceTypes to match order (a list
// of resource-type IDs), leaving anything not mentioned in order (which
// shouldn't normally happen) in whatever order SpecFromConfig already gave
// it, appended at the end.
func reorderResourceTypes(spec *Spec, order []string) {
	if len(order) == 0 {
		return
	}
	byID := make(map[string]ResourceTypeSpec, len(spec.ResourceTypes))
	for _, rt := range spec.ResourceTypes {
		byID[rt.ID] = rt
	}
	reordered := make([]ResourceTypeSpec, 0, len(spec.ResourceTypes))
	seen := make(map[string]bool, len(order))
	for _, id := range order {
		if rt, ok := byID[id]; ok {
			reordered = append(reordered, rt)
			seen[id] = true
		}
	}
	for _, rt := range spec.ResourceTypes {
		if !seen[rt.ID] {
			reordered = append(reordered, rt)
		}
	}
	spec.ResourceTypes = reordered
}

// SpecFromConfig reverses a parsed bsql.Config back into a Studio Spec. It is
// the inverse of Generate: where Generate compiles a Spec's field mappings
// into CEL and assembles a bsql config, SpecFromConfig walks a bsql config
// and reconstructs field mappings from its CEL expressions. Plain column
// references (".<column>") come back as clean column picks; anything else
// comes back as an editable raw-CEL mapping (see celToMapping) rather than an
// attempt to reverse-engineer a recipe.
//
// Known round-trip limitation: id/display_name/description share ONE YAML
// map node (genResourceType's mp), so their relative order within that node
// isn't recoverable from a parsed bsql.Config - SpecFromConfig always
// reconstructs them in the fixed order id, display_name, description. Since
// Generate always emits them in that same canonical order, a Studio-generated
// config (the case the round-trip test proves) regenerates byte-identical.
// A hand-authored config that orders these fields differently (e.g.
// display_name before id) will canonicalize to id-first on import - this is
// a known, narrow loss, not an order-independence guarantee for arbitrary
// input. Trait-scalar fields and profile.<key> fields do NOT have this
// caveat: each lands in its own distinct map/seq entry in genResourceType
// regardless of iteration order, so their relative order to one another (and
// to id/display_name/description) never affects the regenerated YAML.
func SpecFromConfig(cfg *bsql.Config) (*Spec, error) {
	spec := &Spec{
		AppName: cfg.AppName,
		Connect: connectFromConfig(cfg.Connect),
	}

	ids := make([]string, 0, len(cfg.ResourceTypes))
	for id := range cfg.ResourceTypes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		rtSpec, err := resourceTypeFromConfig(id, cfg.ResourceTypes[id])
		if err != nil {
			return nil, err
		}
		spec.ResourceTypes = append(spec.ResourceTypes, *rtSpec)
	}
	return spec, nil
}

func connectFromConfig(dc bsql.DatabaseConfig) ConnectConfig {
	return ConnectConfig{
		Scheme:   dc.Scheme,
		Host:     dc.Host,
		Port:     dc.Port,
		Database: dc.Database,
		User:     dc.User,
		Password: dc.Password,
		Params:   dc.Params,
	}
}

func resourceTypeFromConfig(id string, rt bsql.ResourceType) (*ResourceTypeSpec, error) {
	spec := &ResourceTypeSpec{ID: id, Name: rt.Name}

	var traits *bsql.Traits
	if rt.List != nil && rt.List.Map != nil {
		traits = rt.List.Map.Traits
	}
	spec.Trait = traitKind(traits)

	listSpec, err := listSpecFromConfig(rt.List, traits)
	if err != nil {
		return nil, err
	}
	spec.List = listSpec

	entSpec, err := entitlementsSpecFromConfig(id, rt)
	if err != nil {
		return nil, err
	}
	spec.Entitlements = entSpec

	grants, err := grantsFromConfig(id, rt.Grants)
	if err != nil {
		return nil, err
	}
	spec.Grants = grants

	return spec, nil
}

// traitKind detects which trait (if any) is present on a parsed Traits
// struct, mirroring the trait wrapper key genResourceType emits (user, group,
// role, or app). Returns "none" when no trait mapping is present at all -
// which is also what's reconstructed when a resource type has a trait but no
// trait-specific fields (Generate never emits an empty traits wrapper, so
// there's nothing in the parsed config to detect it from; harmless, since an
// empty traits map has no effect on regenerated YAML either way).
func traitKind(traits *bsql.Traits) string {
	if traits == nil {
		return "none"
	}
	switch {
	case traits.User != nil:
		return "user"
	case traits.Group != nil:
		return "group"
	case traits.Role != nil:
		return "role"
	case traits.App != nil:
		return "app"
	default:
		return "none"
	}
}

func listSpecFromConfig(lq *bsql.ListQuery, traits *bsql.Traits) (ListSpec, error) {
	if lq == nil {
		return ListSpec{}, nil
	}
	var fields []FieldMapping
	if m := lq.Map; m != nil {
		if m.Id != "" {
			fields = append(fields, mustFieldMapping("id", m.Id))
		}
		if m.DisplayName != "" {
			fields = append(fields, mustFieldMapping("display_name", m.DisplayName))
		}
		if m.Description != "" {
			fields = append(fields, mustFieldMapping("description", m.Description))
		}
		fields = append(fields, traitFieldMappings(traits)...)
	}
	return ListSpec{Query: lq.Query, Fields: fields}, nil
}

// traitFieldMappings reverses whichever trait mapping is present (user,
// group, role, or app) into canonical FieldMappings: the fixed set of
// trait-scalar/seq fields (user only) in traitScalarOrder, followed by each
// profile.<key> entry in sorted key order.
func traitFieldMappings(traits *bsql.Traits) []FieldMapping {
	if traits == nil {
		return nil
	}

	var profile map[string]string
	scalars := map[string]string{}

	switch {
	case traits.User != nil:
		u := traits.User
		profile = u.Profile
		if len(u.Emails) > 0 {
			scalars["emails"] = u.Emails[0]
		}
		if u.Status != "" {
			scalars["status"] = u.Status
		}
		if u.AccountType != "" {
			scalars["account_type"] = u.AccountType
		}
		if u.Login != "" {
			scalars["login"] = u.Login
		}
		if len(u.LoginAliases) > 0 {
			scalars["login_aliases"] = u.LoginAliases[0]
		}
		if u.LastLogin != "" {
			scalars["last_login"] = u.LastLogin
		}
		if len(u.EmployeeIDs) > 0 {
			scalars["employee_ids"] = u.EmployeeIDs[0]
		}
		if u.ManagerID != "" {
			scalars["manager_id"] = u.ManagerID
		}
		if u.ManagerEmail != "" {
			scalars["manager_email"] = u.ManagerEmail
		}
		if u.StatusDetails != "" {
			scalars["status_details"] = u.StatusDetails
		}
	case traits.Group != nil:
		profile = traits.Group.Profile
	case traits.Role != nil:
		profile = traits.Role.Profile
	case traits.App != nil:
		profile = traits.App.Profile
	}

	var out []FieldMapping
	for _, key := range traitScalarOrder {
		if cel, ok := scalars[key]; ok {
			out = append(out, mustFieldMapping(key, cel))
		}
	}

	if len(profile) > 0 {
		keys := make([]string, 0, len(profile))
		for k := range profile {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out = append(out, mustFieldMapping("profile."+k, profile[k]))
		}
	}
	return out
}

func entitlementsSpecFromConfig(id string, rt bsql.ResourceType) (EntitlementsSpec, error) {
	if len(rt.StaticEntitlements) > 0 {
		statics := make([]StaticEntitlement, 0, len(rt.StaticEntitlements))
		for _, e := range rt.StaticEntitlements {
			if e == nil {
				continue
			}
			statics = append(statics, StaticEntitlement{
				ID:          e.Id,
				DisplayName: e.DisplayName,
				Description: e.Description,
				Purpose:     e.Purpose,
				GrantableTo: e.GrantableTo,
				Immutable:   e.Immutable,
			})
		}
		return EntitlementsSpec{Mode: "static", Static: statics}, nil
	}

	if rt.Entitlements != nil && rt.Entitlements.Query != "" {
		ent := rt.Entitlements
		// EntitlementsSpec models exactly one entitlement mapping per query
		// (Studio's Fields is a flat []FieldMapping, not per-row). A config
		// with more than one map row can't be reconstructed without silently
		// dropping rows 1..n-1 - fail loudly instead (see the grants path
		// below for the same shape of guard, and the CRITICAL fix this
		// addresses: a real config can have many map rows, e.g.
		// examples/redshift-test.yml's "table" grants query has 12).
		if len(ent.Map) > 1 {
			return EntitlementsSpec{}, fmt.Errorf(
				"resource type %q: this config has a dynamic entitlements query with %d mapping rows; Studio currently supports one entitlement mapping per query and cannot import it without losing data",
				id, len(ent.Map))
		}
		var fields []FieldMapping
		var grantableTo []string
		if len(ent.Map) > 0 && ent.Map[0] != nil {
			mp := ent.Map[0]
			if mp.Id != "" {
				fields = append(fields, mustFieldMapping("id", mp.Id))
			}
			if mp.DisplayName != "" {
				fields = append(fields, mustFieldMapping("display_name", mp.DisplayName))
			}
			if mp.Description != "" {
				fields = append(fields, mustFieldMapping("description", mp.Description))
			}
			if mp.Purpose != "" {
				fields = append(fields, mustFieldMapping("purpose", mp.Purpose))
			}
			if mp.Slug != "" {
				fields = append(fields, mustFieldMapping("slug", mp.Slug))
			}
			grantableTo = mp.GrantableTo
		}
		return EntitlementsSpec{Mode: "query", Query: ent.Query, Fields: fields, GrantableTo: grantableTo}, nil
	}

	return EntitlementsSpec{Mode: "none"}, nil
}

func grantsFromConfig(id string, gqs []*bsql.GrantsQuery) ([]GrantSpec, error) {
	var out []GrantSpec
	for _, gq := range gqs {
		if gq == nil {
			continue
		}

		// GrantSpec models exactly one principal_type/entitlement pair per
		// grants query (Studio's Fields is a flat []FieldMapping, not
		// per-row). A config with more than one map row - a common pattern
		// for fanning one grants query out over multiple principal types
		// and/or entitlements, e.g. examples/redshift-test.yml's "table"
		// type has ONE grants query with 12 map rows - can't be
		// reconstructed without silently dropping rows 1..n-1. Fail loudly
		// instead of truncating.
		if len(gq.Map) > 1 {
			return nil, fmt.Errorf(
				"resource type %q: this config has a grant query with %d mapping rows; Studio currently supports one principal_type per grant query and cannot import it without losing data",
				id, len(gq.Map))
		}

		resourceVar := ""
		if len(gq.Vars) > 0 {
			keys := make([]string, 0, len(gq.Vars))
			for k := range gq.Vars {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if gq.Vars[k] == "resource.ID" {
					resourceVar = k
					break
				}
			}
		}

		principalType := ""
		entitlement := ""
		var fields []FieldMapping
		if len(gq.Map) > 0 && gq.Map[0] != nil {
			row := gq.Map[0]
			principalType = row.PrincipalType
			entitlement = row.Entitlement
			if row.PrincipalId != "" {
				fields = append(fields, mustFieldMapping("principal_id", row.PrincipalId))
			}
			if row.SkipIf != "" {
				fields = append(fields, mustFieldMapping("skip_if", row.SkipIf))
			}
		}

		out = append(out, GrantSpec{
			Query:         gq.Query,
			ResourceVar:   resourceVar,
			PrincipalType: principalType,
			Entitlement:   entitlement,
			Fields:        fields,
		})
	}
	return out, nil
}

// mustFieldMapping builds a FieldMapping for canonical field from a compiled
// CEL expression, via celToMapping.
func mustFieldMapping(field, cel string) FieldMapping {
	column, transform := celToMapping(cel)
	return FieldMapping{Field: field, Column: column, Transform: transform}
}

// celToMapping reverses a compiled CEL expression into either a clean column
// pick (when cel is exactly a plain column reference, ".<column>") or an
// editable raw-CEL transform (everything else). It deliberately does not
// attempt to reverse-engineer which recipe (if any) produced richer CEL -
// that's lossy in general, and a raw-CEL mapping regenerates identically via
// CompileTransform's "raw" case, so it's lossless where it matters: the
// regenerated YAML.
func celToMapping(cel string) (column string, transform *Transform) {
	if m := columnRefRE.FindStringSubmatch(cel); m != nil {
		return m[1], nil
	}
	return "", &Transform{Recipe: RecipeRaw, RawCEL: cel}
}
