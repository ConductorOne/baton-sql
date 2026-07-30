# baton-sql Studio — Engine Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the headless Go engine that compiles a Studio *authoring spec* into a valid, trap-free baton-sql YAML config and validates it authoritatively via `pkg/bsql`.

**Architecture:** A new additive package `pkg/studio` (no changes to existing connector code). It defines a JSON-serializable `Spec` (what the wizard collects), a pure recipe→CEL compiler, a pure Spec→YAML generator that bakes in the silent-failure trap-prevention rules, and a validator that round-trips the generated YAML through baton-sql's own `Parse` + `GetSQLSyncers` + `SQLSyncer.Validate`. A thin CLI (`cmd/baton-sql-studio compile`) exercises the whole engine end-to-end.

**Tech Stack:** Go 1.25.2; `gopkg.in/yaml.v3` (already vendored); reuse `github.com/conductorone/baton-sql/pkg/bsql`, `.../pkg/bcel`, `.../pkg/database`; tests use `modernc.org/sqlite` in-memory (pattern already in `pkg/bsql/provisioning_grant_reject_test.go`).

## Global Constraints

- Module path: `github.com/conductorone/baton-sql` — all imports use this prefix.
- Go version floor: **1.25.2** (from `go.mod`).
- **No AI/LLM** anywhere in the engine — all transforms are deterministic templates.
- **Never modify `pkg/bsql`, `pkg/bcel`, or `pkg/database`** — the engine only consumes their exported API.
- The generator MUST emit only keys it explicitly sets (build an ordered/omitempty output model — do NOT marshal a fully-populated `bsql.Config`, which would serialize empty no-op keys).
- Trap-prevention rules the generator MUST enforce (from the authoring reference):
  1. Never emit `mfa_enabled` / `sso_enabled` (no-op keys).
  2. If `manager_id`/`manager_email` are mapped, guarantee `traits.user.profile` is non-empty (else the manager fields are silently dropped).
  3. Dynamic entitlements MUST always emit `slug`.
  4. `principal_type` in a grant MUST reference a resource-type ID defined in the same Spec.
  5. A resource type with no entitlements AND no grants MUST emit `skip_entitlements_and_grants: true`.
- Every generated YAML MUST round-trip: `bsql.Parse(out)` returns no error, and every produced syncer's `Validate(ctx)` returns no error, for the engine's own golden specs.

---

## File Structure

- `pkg/studio/spec.go` — the `Spec` type tree + JSON tags (the wizard's data model).
- `pkg/studio/recipes.go` — `CompileTransform` / `CompileField`: deterministic recipe→CEL.
- `pkg/studio/generate.go` — `Generate(spec) ([]byte, error)`: Spec→YAML with trap-prevention.
- `pkg/studio/validate.go` — `Validate(ctx, spec, opts) (*Report, error)`: spec checks + authoritative round-trip.
- `pkg/studio/preview.go` — `PreviewField(ctx, env, fm, row)`: evaluate a mapping against one sample row via `bcel`.
- `pkg/studio/*_test.go` — one test file per source file.
- `cmd/baton-sql-studio/main.go` — CLI: `compile <spec.json>` → validated YAML on stdout.
- `pkg/studio/testdata/finance.spec.json` + `pkg/studio/testdata/finance.golden.yaml` — golden fixtures.

---

## Task 1: Spec types + JSON round-trip

**Files:**
- Create: `pkg/studio/spec.go`
- Test: `pkg/studio/spec_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: the exported types below — every later task builds on these exact names.

```go
package studio

type Spec struct {
	AppName       string             `json:"app_name"`
	Connect       ConnectConfig      `json:"connect"`
	ResourceTypes []ResourceTypeSpec `json:"resource_types"`
}

type ConnectConfig struct {
	Scheme   string            `json:"scheme"`
	Host     string            `json:"host"`
	Port     string            `json:"port"`
	Database string            `json:"database"`
	User     string            `json:"user"`
	Password string            `json:"password"`
	Params   map[string]string `json:"params,omitempty"`
}

type ResourceTypeSpec struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Trait        string           `json:"trait"` // user|group|role|app|none
	List         ListSpec         `json:"list"`
	Entitlements EntitlementsSpec `json:"entitlements"`
	Grants       []GrantSpec      `json:"grants,omitempty"`
}

type ListSpec struct {
	Query  string         `json:"query"`
	Fields []FieldMapping `json:"fields"`
}

type FieldMapping struct {
	Field     string     `json:"field"`            // canonical, e.g. "id","display_name","emails","status","profile.department"
	Column    string     `json:"column,omitempty"` // source column when no/simple transform
	Transform *Transform `json:"transform,omitempty"`
}

type Transform struct {
	Recipe string         `json:"recipe"`           // see recipes.go
	Args   map[string]any `json:"args,omitempty"`
	RawCEL string         `json:"raw_cel,omitempty"`
}

type EntitlementsSpec struct {
	Mode   string              `json:"mode"` // "static" | "query" | "none"
	Static []StaticEntitlement `json:"static,omitempty"`
	Query  string              `json:"query,omitempty"`
	Fields []FieldMapping      `json:"fields,omitempty"` // for query mode
}

type StaticEntitlement struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Purpose     string `json:"purpose,omitempty"`
}

type GrantSpec struct {
	Query         string         `json:"query"`
	ResourceVar   string         `json:"resource_var,omitempty"`   // ?<var> bound to resource.ID
	PrincipalType string         `json:"principal_type"`
	Entitlement   string         `json:"entitlement"`              // entitlement id/slug this grant targets
	Fields        []FieldMapping `json:"fields"`                   // principal_id, resource_id, skip_if
}
```

- [ ] **Step 1: Write the failing test**

```go
package studio

import (
	"encoding/json"
	"testing"
)

func TestSpec_JSONRoundTrip(t *testing.T) {
	in := Spec{
		AppName: "Finance DB",
		Connect: ConnectConfig{Scheme: "mysql", Host: "db", Port: "3306", Database: "finance"},
		ResourceTypes: []ResourceTypeSpec{{
			ID: "users", Name: "Users", Trait: "user",
			List: ListSpec{Query: "SELECT id FROM employees", Fields: []FieldMapping{{Field: "id", Column: "id"}}},
		}},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Spec
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.AppName != in.AppName || out.ResourceTypes[0].List.Fields[0].Field != "id" {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/studio/ -run TestSpec_JSONRoundTrip -v`
Expected: FAIL — package/types not defined.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/studio/spec.go` with exactly the types in the Interfaces block above.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/studio/ -run TestSpec_JSONRoundTrip -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/studio/spec.go pkg/studio/spec_test.go
git commit -m "feat(studio): authoring spec types with JSON round-trip"
```

---

## Task 2: Recipe compiler — column ref + simple recipes

**Files:**
- Create: `pkg/studio/recipes.go`
- Test: `pkg/studio/recipes_test.go`

**Interfaces:**
- Consumes: `FieldMapping`, `Transform` (Task 1).
- Produces:
  - `func CompileField(fm FieldMapping) (string, error)` — returns a CEL expression string for the field's value.
  - `func CompileTransform(t *Transform, column string) (string, error)`.
  - Recipe name constants: `RecipeSlugify = "slugify"`, `RecipeTitleCase = "title_case"`, `RecipeCoerceString = "coerce_string"`, `RecipeNullDefault = "null_default"`, `RecipeCompositeID = "composite_id"`, `RecipeStatusTernary = "status_ternary"`, `RecipeAccountTypeTernary = "account_type_ternary"`, `RecipeRaw = "raw"`.

Note: baton-sql CEL references a column as `.<column>` (leading dot; see reference examples like `".role_name"`). A bare column ref compiles to `"." + column`.

- [ ] **Step 1: Write the failing test**

```go
package studio

import "testing"

func TestCompileField_SimpleRecipes(t *testing.T) {
	cases := []struct {
		name string
		fm   FieldMapping
		want string
	}{
		{"plain column", FieldMapping{Field: "id", Column: "id"}, ".id"},
		{"coerce string", FieldMapping{Field: "id", Column: "user_id", Transform: &Transform{Recipe: RecipeCoerceString}}, "string(.user_id)"},
		{"null default", FieldMapping{Field: "last_login", Column: "last_login", Transform: &Transform{Recipe: RecipeNullDefault}}, ".last_login != null ? string(.last_login) : ''"},
		{"slugify", FieldMapping{Field: "slug", Column: "role_name", Transform: &Transform{Recipe: RecipeSlugify}}, "slugify(.role_name)"},
		{"title case", FieldMapping{Field: "display_name", Column: "role_name", Transform: &Transform{Recipe: RecipeTitleCase}}, "titleCase(.role_name)"},
		{"raw cel", FieldMapping{Field: "id", Transform: &Transform{Recipe: RecipeRaw, RawCEL: ".a + .b"}}, ".a + .b"},
	}
	for _, c := range cases {
		got, err := CompileField(c.fm)
		if err != nil {
			t.Fatalf("%s: err %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/studio/ -run TestCompileField_SimpleRecipes -v`
Expected: FAIL — `CompileField` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
package studio

import "fmt"

const (
	RecipeSlugify            = "slugify"
	RecipeTitleCase          = "title_case"
	RecipeCoerceString       = "coerce_string"
	RecipeNullDefault        = "null_default"
	RecipeCompositeID        = "composite_id"
	RecipeStatusTernary      = "status_ternary"
	RecipeAccountTypeTernary = "account_type_ternary"
	RecipeRaw                = "raw"
)

func colRef(column string) string { return "." + column }

func CompileField(fm FieldMapping) (string, error) {
	if fm.Transform == nil {
		if fm.Column == "" {
			return "", fmt.Errorf("field %q: no column and no transform", fm.Field)
		}
		return colRef(fm.Column), nil
	}
	return CompileTransform(fm.Transform, fm.Column)
}

func CompileTransform(t *Transform, column string) (string, error) {
	switch t.Recipe {
	case RecipeRaw:
		if t.RawCEL == "" {
			return "", fmt.Errorf("raw recipe requires raw_cel")
		}
		return t.RawCEL, nil
	case RecipeCoerceString:
		return fmt.Sprintf("string(%s)", colRef(column)), nil
	case RecipeNullDefault:
		return fmt.Sprintf("%s != null ? string(%s) : ''", colRef(column), colRef(column)), nil
	case RecipeSlugify:
		return fmt.Sprintf("slugify(%s)", colRef(column)), nil
	case RecipeTitleCase:
		return fmt.Sprintf("titleCase(%s)", colRef(column)), nil
	default:
		return "", fmt.Errorf("unknown or non-simple recipe %q", t.Recipe)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/studio/ -run TestCompileField_SimpleRecipes -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/studio/recipes.go pkg/studio/recipes_test.go
git commit -m "feat(studio): recipe compiler — column ref and simple recipes"
```

---

## Task 3: Recipe compiler — composite & ternary recipes

**Files:**
- Modify: `pkg/studio/recipes.go`
- Test: `pkg/studio/recipes_test.go` (add cases)

**Interfaces:**
- Consumes: everything from Task 2.
- Produces: extends `CompileTransform` to handle `composite_id`, `status_ternary`, `account_type_ternary` using `Transform.Args`:
  - `composite_id`: `Args["columns"] []any` (column names), `Args["sep"] string` (default `.`) → `string(.a) + '.' + string(.b) + ...`.
  - `status_ternary`: `Args["column"] string`, `Args["enabled"] []any` (raw values meaning enabled) → nested ternary yielding `'enabled'`/`'disabled'`.
  - `account_type_ternary`: `Args["column"] string`, `Args["system_prefix"] string` → `string(.col).startsWith('<prefix>') ? 'system' : 'human'`.

- [ ] **Step 1: Write the failing test**

```go
func TestCompileTransform_CompositeAndTernary(t *testing.T) {
	comp := &Transform{Recipe: RecipeCompositeID, Args: map[string]any{
		"columns": []any{"database_name", "schema_name", "table_name"}, "sep": ".",
	}}
	got, err := CompileTransform(comp, "")
	if err != nil {
		t.Fatal(err)
	}
	want := "string(.database_name) + '.' + string(.schema_name) + '.' + string(.table_name)"
	if got != want {
		t.Errorf("composite: got %q want %q", got, want)
	}

	st := &Transform{Recipe: RecipeStatusTernary, Args: map[string]any{
		"column": "status", "enabled": []any{"1"},
	}}
	got, err = CompileTransform(st, "status")
	if err != nil {
		t.Fatal(err)
	}
	if got != "string(.status) == '1' ? 'enabled' : 'disabled'" {
		t.Errorf("status ternary: got %q", got)
	}

	at := &Transform{Recipe: RecipeAccountTypeTernary, Args: map[string]any{"system_prefix": "_SYS"}}
	got, err = CompileTransform(at, "user_name")
	if err != nil {
		t.Fatal(err)
	}
	if got != "string(.user_name).startsWith('_SYS') ? 'system' : 'human'" {
		t.Errorf("account_type ternary: got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/studio/ -run TestCompileTransform_CompositeAndTernary -v`
Expected: FAIL — recipes fall through to "unknown".

- [ ] **Step 3: Write minimal implementation**

Add these cases inside `CompileTransform`'s switch (before `default`). Include a small helper `argString(args, key, def)` and `argStrings(args, key)`.

```go
	case RecipeCompositeID:
		cols := argStrings(t.Args, "columns")
		if len(cols) == 0 {
			return "", fmt.Errorf("composite_id requires args.columns")
		}
		sep := argString(t.Args, "sep", ".")
		parts := make([]string, 0, len(cols))
		for _, c := range cols {
			parts = append(parts, fmt.Sprintf("string(%s)", colRef(c)))
		}
		return strings.Join(parts, fmt.Sprintf(" + '%s' + ", sep)), nil
	case RecipeStatusTernary:
		col := argString(t.Args, "column", column)
		enabled := argStrings(t.Args, "enabled")
		if col == "" || len(enabled) == 0 {
			return "", fmt.Errorf("status_ternary requires args.column and args.enabled")
		}
		conds := make([]string, 0, len(enabled))
		for _, v := range enabled {
			conds = append(conds, fmt.Sprintf("string(%s) == '%s'", colRef(col), v))
		}
		return fmt.Sprintf("%s ? 'enabled' : 'disabled'", strings.Join(conds, " || ")), nil
	case RecipeAccountTypeTernary:
		col := argString(t.Args, "column", column)
		prefix := argString(t.Args, "system_prefix", "")
		if col == "" || prefix == "" {
			return "", fmt.Errorf("account_type_ternary requires args.column and args.system_prefix")
		}
		return fmt.Sprintf("string(%s).startsWith('%s') ? 'system' : 'human'", colRef(col), prefix), nil
```

Add helpers and the `strings` import:

```go
func argString(args map[string]any, key, def string) string {
	if v, ok := args[key].(string); ok && v != "" {
		return v
	}
	return def
}

func argStrings(args map[string]any, key string) []string {
	raw, ok := args[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		if s, ok := r.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/studio/ -run 'TestCompile' -v`
Expected: PASS (both recipe tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/studio/recipes.go pkg/studio/recipes_test.go
git commit -m "feat(studio): composite-id and ternary recipes"
```

---

## Task 4: YAML generator — users/list block with trap-prevention

**Files:**
- Create: `pkg/studio/generate.go`
- Test: `pkg/studio/generate_test.go`

**Interfaces:**
- Consumes: `Spec`, `CompileField` (Tasks 1–3).
- Produces:
  - `func Generate(spec *Spec) ([]byte, error)` — full YAML (this task only needs the user resource type to pass; later tasks extend entitlements/grants).
  - Internally uses `yaml.v3` with an ordered output model built from `yaml.MapSlice`-style maps so only set keys appear.

Trap rules enforced here: no `mfa_enabled`/`sso_enabled`; manager fields force a non-empty `profile`; `skip_entitlements_and_grants: true` when a type has no entitlements and no grants.

- [ ] **Step 1: Write the failing test**

```go
package studio

import (
	"strings"
	"testing"

	"github.com/conductorone/baton-sql/pkg/bsql"
)

func TestGenerate_UsersListParsesAndMapsTraits(t *testing.T) {
	spec := &Spec{
		AppName: "Finance DB",
		Connect: ConnectConfig{Scheme: "mysql", Host: "db", Port: "3306", Database: "finance", User: "svc", Password: "pw"},
		ResourceTypes: []ResourceTypeSpec{{
			ID: "users", Name: "Users", Trait: "user",
			List: ListSpec{
				Query: "SELECT id, email, first_name, last_name, manager_id FROM employees",
				Fields: []FieldMapping{
					{Field: "id", Column: "id"},
					{Field: "display_name", Transform: &Transform{Recipe: RecipeCompositeID, Args: map[string]any{"columns": []any{"first_name", "last_name"}, "sep": " "}}},
					{Field: "emails", Column: "email"},
					{Field: "manager_id", Column: "manager_id"},
				},
			},
			Entitlements: EntitlementsSpec{Mode: "none"},
		}},
	}
	out, err := Generate(spec)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// 1. Must parse via baton-sql's own parser.
	if _, err := bsql.Parse(out); err != nil {
		t.Fatalf("bsql.Parse rejected generated yaml: %v\n---\n%s", err, out)
	}
	s := string(out)
	// 2. Trap: no no-op keys.
	if strings.Contains(s, "mfa_enabled") || strings.Contains(s, "sso_enabled") {
		t.Errorf("generated no-op key; yaml:\n%s", s)
	}
	// 3. Trap: manager present => profile non-empty.
	if !strings.Contains(s, "profile:") {
		t.Errorf("manager mapped but no profile block emitted; yaml:\n%s", s)
	}
	// 4. Trap: no E&G => skip flag.
	if !strings.Contains(s, "skip_entitlements_and_grants: true") {
		t.Errorf("expected skip_entitlements_and_grants for E&G-less type; yaml:\n%s", s)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/studio/ -run TestGenerate_UsersListParsesAndMapsTraits -v`
Expected: FAIL — `Generate` undefined.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/studio/generate.go`. Build an ordered map (`yaml.Node` with `!!map`) so only set keys serialize. Key logic (abbreviated to the structural rules — fill in each trait field the same way as `id`/`display_name`):

```go
package studio

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

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

	// Trap #5: no entitlements and no grants => skip flag.
	if rt.Entitlements.Mode == "none" && len(rt.Grants) == 0 {
		putScalar(n, "skip_entitlements_and_grants", "true")
	}
	return n, nil
}
```

Add the small yaml-node helpers (`mapNode`, `putScalar`, `putNode`, `putSeq`, `profileKey`). `putScalar` for the boolean `skip_entitlements_and_grants` must emit an unquoted `!!bool`; give it a variant `putBool`. Example helper set:

```go
func mapNode() *yaml.Node { return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"} }

func putScalar(m *yaml.Node, key, val string) {
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Value: val})
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
```

Replace the `skip_entitlements_and_grants` line to emit a real bool:

```go
	if rt.Entitlements.Mode == "none" && len(rt.Grants) == 0 {
		m := n
		m.Content = append(m.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "skip_entitlements_and_grants"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
	}
```

(Handle the unused `fmt` import by removing it if not used, or keep it for later tasks.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/studio/ -run TestGenerate_UsersListParsesAndMapsTraits -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/studio/generate.go pkg/studio/generate_test.go
git commit -m "feat(studio): YAML generator for user list block with trap-prevention"
```

---

## Task 5: YAML generator — entitlements (static + dynamic, always slug)

**Files:**
- Modify: `pkg/studio/generate.go`
- Test: `pkg/studio/generate_test.go` (add)

**Interfaces:**
- Consumes: Task 4.
- Produces: `genResourceType` now emits `static_entitlements:` (for `Mode=="static"`) or a dynamic `entitlements:` block (for `Mode=="query"`). Dynamic entries ALWAYS include `slug` (Trap #3): if the spec maps a `slug` field use it, else default `slug` to `slugify(display_name-cel)` — reuse `RecipeSlugify` by wrapping the compiled display_name; if display_name is a plain column, slug = `slugify(.<col>)`.

- [ ] **Step 1: Write the failing test**

```go
func TestGenerate_DynamicEntitlementsAlwaysSlug(t *testing.T) {
	spec := &Spec{
		AppName: "EBS",
		ResourceTypes: []ResourceTypeSpec{{
			ID: "menu", Name: "Menu", Trait: "role",
			List: ListSpec{Query: "SELECT menu_id, menu_name FROM menus", Fields: []FieldMapping{
				{Field: "id", Column: "menu_id"}, {Field: "display_name", Column: "menu_name"},
			}},
			Entitlements: EntitlementsSpec{
				Mode:  "query",
				Query: "SELECT function_id, function_name FROM functions WHERE menu_id = ?<menu_id>",
				Fields: []FieldMapping{
					{Field: "id", Column: "function_id"},
					{Field: "display_name", Column: "function_name"},
				},
			},
		}},
	}
	out, err := Generate(spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bsql.Parse(out); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "slug:") {
		t.Errorf("dynamic entitlements must always emit slug; yaml:\n%s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/studio/ -run TestGenerate_DynamicEntitlementsAlwaysSlug -v`
Expected: FAIL — no entitlements emitted / no `slug:`.

- [ ] **Step 3: Write minimal implementation**

In `genResourceType`, after the list block, add entitlement generation:

```go
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
		var displayCELForSlug string
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
			if fm.Field == "display_name" {
				displayCELForSlug = cel
			}
		}
		// Trap #3: always emit slug.
		if !hasKey(mp, "slug") {
			putScalar(mp, "slug", fmt.Sprintf("slugify(%s)", displayCELForSlug))
		}
		putNode(ent, "map", mp)
		putNode(n, "entitlements", ent)
	}
```

Add `hasKey`:

```go
func hasKey(m *yaml.Node, key string) bool {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return true
		}
	}
	return false
}
```

(Restore the `fmt` import used here.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/studio/ -run 'TestGenerate' -v`
Expected: PASS (all generate tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/studio/generate.go pkg/studio/generate_test.go
git commit -m "feat(studio): static and dynamic entitlement generation (always slug)"
```

---

## Task 6: YAML generator — resource-scoped grants

**Files:**
- Modify: `pkg/studio/generate.go`
- Test: `pkg/studio/generate_test.go` (add)

**Interfaces:**
- Consumes: Task 5.
- Produces: `genResourceType` emits a `grants:` sequence. Each `GrantSpec` becomes `{query, vars:{<ResourceVar>: "resource.ID"}, map:[{...}]}`. `principal_id` → `map[].principal_id`; `principal_type` → literal string (Trap #4: caller-validated in Task 7's validator); optional `skip_if`. If `ResourceVar` set, emit the `vars` binding.

- [ ] **Step 1: Write the failing test**

```go
func TestGenerate_ResourceScopedGrant(t *testing.T) {
	spec := &Spec{
		AppName: "Finance DB",
		ResourceTypes: []ResourceTypeSpec{
			{ID: "users", Name: "Users", Trait: "user",
				List: ListSpec{Query: "SELECT id FROM employees", Fields: []FieldMapping{{Field: "id", Column: "id"}, {Field: "display_name", Column: "id"}}},
				Entitlements: EntitlementsSpec{Mode: "none"}},
			{ID: "roles", Name: "Roles", Trait: "role",
				List: ListSpec{Query: "SELECT role_id, role_name FROM roles", Fields: []FieldMapping{{Field: "id", Column: "role_id"}, {Field: "display_name", Column: "role_name"}}},
				Entitlements: EntitlementsSpec{Mode: "static", Static: []StaticEntitlement{{ID: "assigned", DisplayName: "Assigned", Purpose: "assignment"}}},
				Grants: []GrantSpec{{
					Query: "SELECT user_id FROM user_roles WHERE role_id = ?<role_id>",
					ResourceVar: "role_id", PrincipalType: "users", Entitlement: "assigned",
					Fields: []FieldMapping{{Field: "principal_id", Column: "user_id"}},
				}},
			},
		},
	}
	out, err := Generate(spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bsql.Parse(out); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, `role_id: resource.ID`) && !strings.Contains(s, `role_id: "resource.ID"`) {
		t.Errorf("expected resource-scoped var binding; yaml:\n%s", s)
	}
	if !strings.Contains(s, "principal_type: users") {
		t.Errorf("expected principal_type users; yaml:\n%s", s)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/studio/ -run TestGenerate_ResourceScopedGrant -v`
Expected: FAIL — no grants block.

- [ ] **Step 3: Write minimal implementation**

In `genResourceType`, after entitlements:

```go
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
			row := mapNode()
			for _, fm := range g.Fields {
				cel, err := CompileField(fm)
				if err != nil {
					return nil, err
				}
				switch fm.Field {
				case "principal_id", "skip_if", "resource_id":
					putScalar(row, fm.Field, cel)
				}
			}
			putScalar(row, "principal_type", g.PrincipalType)
			if g.Entitlement != "" {
				putScalar(row, "entitlement_id", g.Entitlement)
			}
			mseq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			mseq.Content = append(mseq.Content, row)
			putNode(gm, "map", mseq)
			gseq.Content = append(gseq.Content, gm)
		}
		putNode(n, "grants", gseq)
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/studio/ -run 'TestGenerate' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/studio/generate.go pkg/studio/generate_test.go
git commit -m "feat(studio): resource-scoped grant generation"
```

---

## Task 7: Authoritative validation via pkg/bsql + Report

**Files:**
- Create: `pkg/studio/validate.go`
- Test: `pkg/studio/validate_test.go`

**Interfaces:**
- Consumes: `Spec`, `Generate` (Tasks 1–6); baton-sql exported API: `bsql.Parse`, `Config.GetSQLSyncers`, `SQLSyncer.Validate`; `bcel.NewEnv`.
- Produces:
  - `type Report struct { OK bool; Errors []Issue }`; `type Issue struct { Scope, Field, Message string }`.
  - `type ValidateOptions struct { DB *sql.DB; DBEngine database.DbEngine }` (nil DB => use an in-memory sqlite handle for static-only validation).
  - `func Validate(ctx context.Context, spec *Spec, opts ValidateOptions) (*Report, error)`.

Validation steps: (1) spec-level: every `GrantSpec.PrincipalType` matches a defined `ResourceTypeSpec.ID` (Trap #4). (2) `Generate` the YAML. (3) `bsql.Parse`. (4) build `dbs`, `celEnv := bcel.NewEnv(ctx)`, `syncers := cfg.GetSQLSyncers(...)`; call `Validate(ctx)` on each; collect errors into `Report`.

- [ ] **Step 1: Write the failing test**

```go
package studio

import (
	"context"
	"testing"
)

func TestValidate_BadPrincipalTypeReported(t *testing.T) {
	spec := &Spec{
		AppName: "x",
		ResourceTypes: []ResourceTypeSpec{{
			ID: "roles", Name: "Roles", Trait: "role",
			List: ListSpec{Query: "SELECT role_id FROM roles", Fields: []FieldMapping{{Field: "id", Column: "role_id"}, {Field: "display_name", Column: "role_id"}}},
			Entitlements: EntitlementsSpec{Mode: "static", Static: []StaticEntitlement{{ID: "assigned", DisplayName: "Assigned"}}},
			Grants: []GrantSpec{{Query: "SELECT u FROM t", PrincipalType: "does_not_exist", Entitlement: "assigned", Fields: []FieldMapping{{Field: "principal_id", Column: "u"}}}},
		}},
	}
	rep, err := Validate(context.Background(), spec, ValidateOptions{})
	if err != nil {
		t.Fatalf("validate returned error: %v", err)
	}
	if rep.OK {
		t.Fatal("expected report NOT ok for undefined principal_type")
	}
	found := false
	for _, is := range rep.Errors {
		if is.Field == "principal_type" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a principal_type issue, got %+v", rep.Errors)
	}
}

func TestValidate_GoodSpecOK(t *testing.T) {
	spec := goldenFinanceSpec(t) // helper reads testdata/finance.spec.json (added in Task 9)
	rep, err := Validate(context.Background(), spec, ValidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK {
		t.Fatalf("golden spec should validate; issues: %+v", rep.Errors)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/studio/ -run TestValidate_BadPrincipalTypeReported -v`
Expected: FAIL — `Validate`/`Report` undefined. (The `GoodSpecOK` test also fails until Task 9 adds the fixture + helper — that is expected; run only the first test now.)

- [ ] **Step 3: Write minimal implementation**

```go
package studio

import (
	"context"
	"database/sql"

	_ "modernc.org/sqlite"

	"github.com/conductorone/baton-sql/pkg/bcel"
	"github.com/conductorone/baton-sql/pkg/bsql"
	"github.com/conductorone/baton-sql/pkg/database"
)

type Issue struct {
	Scope   string `json:"scope"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

type Report struct {
	OK     bool    `json:"ok"`
	Errors []Issue `json:"errors"`
}

type ValidateOptions struct {
	DB       *sql.DB
	DBEngine database.DbEngine
}

func Validate(ctx context.Context, spec *Spec, opts ValidateOptions) (*Report, error) {
	rep := &Report{OK: true}

	// (1) Spec-level: principal_type references a defined resource type.
	defined := map[string]bool{}
	for _, rt := range spec.ResourceTypes {
		defined[rt.ID] = true
	}
	for _, rt := range spec.ResourceTypes {
		for _, g := range rt.Grants {
			if !defined[g.PrincipalType] {
				rep.OK = false
				rep.Errors = append(rep.Errors, Issue{Scope: rt.ID, Field: "principal_type",
					Message: "principal_type \"" + g.PrincipalType + "\" is not a defined resource type"})
			}
		}
	}

	// (2)+(3) Generate + parse via baton-sql's own parser.
	out, err := Generate(spec)
	if err != nil {
		rep.OK = false
		rep.Errors = append(rep.Errors, Issue{Field: "generate", Message: err.Error()})
		return rep, nil
	}
	cfg, err := bsql.Parse(out)
	if err != nil {
		rep.OK = false
		rep.Errors = append(rep.Errors, Issue{Field: "parse", Message: err.Error()})
		return rep, nil
	}

	// (4) Authoritative static validation through GetSQLSyncers + Validate.
	db := opts.DB
	if db == nil {
		db, err = sql.Open("sqlite", ":memory:")
		if err != nil {
			return nil, err
		}
		defer db.Close()
	}
	dbs := map[string]*sql.DB{"studio": db}
	celEnv, err := bcel.NewEnv(ctx)
	if err != nil {
		return nil, err
	}
	syncers, err := cfg.GetSQLSyncers(ctx, dbs, opts.DBEngine, celEnv)
	if err != nil {
		rep.OK = false
		rep.Errors = append(rep.Errors, Issue{Field: "config", Message: err.Error()})
		return rep, nil
	}
	for _, sy := range syncers {
		if v, ok := sy.(interface{ Validate(context.Context) error }); ok {
			if verr := v.Validate(ctx); verr != nil {
				rep.OK = false
				rep.Errors = append(rep.Errors, Issue{Field: "validate", Message: verr.Error()})
			}
		}
	}
	return rep, nil
}
```

Note: the `interface{ Validate(context.Context) error }` assertion avoids importing the concrete `*SQLSyncer` type; `GetSQLSyncers` returns `connectorbuilder.ResourceSyncer`, and `*SQLSyncer` satisfies this method set (confirmed at `pkg/bsql/sql_syncer.go:149`).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/studio/ -run TestValidate_BadPrincipalTypeReported -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/studio/validate.go pkg/studio/validate_test.go
git commit -m "feat(studio): authoritative validation via pkg/bsql with report"
```

---

## Task 8: Field preview via bcel (live sample-row evaluation)

**Files:**
- Create: `pkg/studio/preview.go`
- Test: `pkg/studio/preview_test.go`

**Interfaces:**
- Consumes: `FieldMapping`, `CompileField` (Tasks 1–3); `bcel.NewEnv`, `(*bcel.Env).EvaluateString`.
- Produces: `func PreviewField(ctx context.Context, env *bcel.Env, fm FieldMapping, row map[string]any) (string, error)` — compiles the field to CEL then evaluates it against one sample row, returning the string result the UI shows as "→ value".

- [ ] **Step 1: Write the failing test**

```go
package studio

import (
	"context"
	"testing"

	"github.com/conductorone/baton-sql/pkg/bcel"
)

func TestPreviewField_Composite(t *testing.T) {
	ctx := context.Background()
	env, err := bcel.NewEnv(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fm := FieldMapping{Field: "display_name", Transform: &Transform{Recipe: RecipeCompositeID,
		Args: map[string]any{"columns": []any{"first_name", "last_name"}, "sep": " "}}}
	got, err := PreviewField(ctx, env, fm, map[string]any{"first_name": "Ada", "last_name": "Lovelace"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Ada Lovelace" {
		t.Fatalf("preview: got %q want %q", got, "Ada Lovelace")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/studio/ -run TestPreviewField_Composite -v`
Expected: FAIL — `PreviewField` undefined. (If `bcel.EvaluateString` expects a specific input-shaping call such as `SyncInputs`, adapt using the helpers at `pkg/bcel/bcel.go:109`; the test tells you if the raw row needs wrapping.)

- [ ] **Step 3: Write minimal implementation**

```go
package studio

import (
	"context"

	"github.com/conductorone/baton-sql/pkg/bcel"
)

func PreviewField(ctx context.Context, env *bcel.Env, fm FieldMapping, row map[string]any) (string, error) {
	expr, err := CompileField(fm)
	if err != nil {
		return "", err
	}
	inputs := env.SyncInputs(row) // shape a DB row into CEL inputs (bcel.go:109)
	return env.EvaluateString(ctx, expr, inputs)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/studio/ -run TestPreviewField_Composite -v`
Expected: PASS. If `SyncInputs`/`EvaluateString` signatures differ, align the call to the real signatures at `pkg/bcel/bcel.go:71,109` (do not change bcel).

- [ ] **Step 5: Commit**

```bash
git add pkg/studio/preview.go pkg/studio/preview_test.go
git commit -m "feat(studio): live field preview via bcel"
```

---

## Task 9: CLI + end-to-end golden fixture

**Files:**
- Create: `cmd/baton-sql-studio/main.go`
- Create: `pkg/studio/testdata/finance.spec.json`
- Create: `pkg/studio/testdata/finance.golden.yaml`
- Modify: `pkg/studio/validate_test.go` (add `goldenFinanceSpec` helper + enable `TestValidate_GoodSpecOK`)
- Test: `cmd/baton-sql-studio/main_test.go`

**Interfaces:**
- Consumes: `Spec`, `Generate`, `Validate` (Tasks 1–7).
- Produces: CLI `baton-sql-studio compile <spec.json>` that prints validated YAML to stdout and exits non-zero (printing issues to stderr) if the report is not OK. Test helper `goldenFinanceSpec(t *testing.T) *Spec` reads the fixture.

- [ ] **Step 1: Write the fixture spec**

Create `pkg/studio/testdata/finance.spec.json` — a realistic multi-resource-type spec (users + roles with a resource-scoped grant + static entitlement). Model it on the mockup's Finance DB (users list with composite display_name + status ternary + profile.department + manager_id; roles list; roles grant `WHERE role_id = ?<role_id>` principal_type=users). Keep queries syntactically simple (they are only statically validated).

- [ ] **Step 2: Write the failing CLI test**

```go
package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCLI_CompileGolden(t *testing.T) {
	out, err := exec.Command("go", "run", ".", "compile", "../../pkg/studio/testdata/finance.spec.json").CombinedOutput()
	if err != nil {
		t.Fatalf("compile failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "resource_types:") {
		t.Fatalf("expected yaml output, got:\n%s", out)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./cmd/baton-sql-studio/ -run TestCLI_CompileGolden -v`
Expected: FAIL — no `main` / no `compile` subcommand.

- [ ] **Step 4: Write minimal implementation**

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/conductorone/baton-sql/pkg/studio"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "compile" {
		fmt.Fprintln(os.Stderr, "usage: baton-sql-studio compile <spec.json>")
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var spec studio.Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		fmt.Fprintln(os.Stderr, "bad spec json:", err)
		os.Exit(1)
	}
	ctx := context.Background()
	rep, err := studio.Validate(ctx, &spec, studio.ValidateOptions{})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out, err := studio.Generate(&spec)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(string(out))
	if !rep.OK {
		for _, is := range rep.Errors {
			fmt.Fprintf(os.Stderr, "! [%s] %s: %s\n", is.Scope, is.Field, is.Message)
		}
		os.Exit(1)
	}
}
```

- [ ] **Step 5: Add the golden-spec helper and enable the OK test**

In `pkg/studio/validate_test.go`:

```go
func goldenFinanceSpec(t *testing.T) *Spec {
	t.Helper()
	data, err := os.ReadFile("testdata/finance.spec.json")
	if err != nil {
		t.Fatal(err)
	}
	var s Spec
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	return &s
}
```

(Add `encoding/json` and `os` imports.)

- [ ] **Step 6: Run the full package + CLI tests**

Run: `go test ./pkg/studio/... ./cmd/baton-sql-studio/... -v`
Expected: PASS, including `TestValidate_GoodSpecOK`. Also run `go build ./...` and `go vet ./pkg/studio/...`.

- [ ] **Step 7: Regenerate the golden yaml and commit**

Run: `go run ./cmd/baton-sql-studio compile pkg/studio/testdata/finance.spec.json > pkg/studio/testdata/finance.golden.yaml`
Eyeball `finance.golden.yaml`: confirm multi-resource-type, resource-scoped `vars: {role_id: resource.ID}`, no `mfa_enabled`, a `profile:` under users, and `slug:` on any dynamic entitlements.

```bash
git add cmd/baton-sql-studio pkg/studio/testdata pkg/studio/validate_test.go
git commit -m "feat(studio): compile CLI and end-to-end golden fixture"
```

---

## Self-Review

**Spec coverage (against the design doc §4–§8):**
- Resource-type-as-unit model → Tasks 1, 4–6 (repeatable `ResourceTypeSpec`; generator loops resource types). ✓
- Alias fast-path + transform recipes + raw CEL → Tasks 2–3 (`CompileField`/`CompileTransform`, incl. `raw`). ✓
- Recipes seeded from surveyed patterns (slugify, composite id, status ternary, titleCase, coerce, null-default, account-type ternary) → Tasks 2–3. ✓
- Static AND dynamic entitlements, always slug → Task 5. ✓
- Resource-scoped grants, principal_type constrained → Tasks 6 (generation) + 7 (validation). ✓
- Trap-prevention (no-op keys, manager→profile, slug, principal_type, skip flag) → Tasks 4–7 (asserted in tests). ✓
- Authoritative validation via pkg/bsql (no reimplementation) → Task 7. ✓
- Live field preview via the same CEL runtime → Task 8. ✓
- **Not in this plan (by design — later plans):** live DB connection & query-running against a customer DB, the HTTP server, and the web wizard UI (Plans 2 and 3). Pagination generation and `expandable` are also deferred (design §8 fast-follows) — noted, not silently dropped.

**Placeholder scan:** no TBD/TODO; every code step has real code. The one forward-reference (`goldenFinanceSpec` used in Task 7's `TestValidate_GoodSpecOK`) is explicitly defined in Task 9 Step 5, and Task 7 Step 2 instructs running only the first test until then.

**Type consistency:** `Spec`/`FieldMapping`/`Transform`/`ResourceTypeSpec`/`EntitlementsSpec`/`GrantSpec` names are used identically across Tasks 1–9; `CompileField`/`CompileTransform`, `Generate`, `Validate`/`Report`/`Issue`/`ValidateOptions`, `PreviewField` signatures match between producer and consumer tasks.

---

## Follow-on plans (outline — to be written next, each its own file)

**Plan 2 — Local server + live query runner** (`pkg/studio/server`, `cmd/baton-sql-studio serve`):
- Connect endpoint using `database.Connect(ctx, ConnectOptions)` → hold `*sql.DB` + `DbEngine` per session; "test connection".
- Run-query endpoint: execute an arbitrary SELECT (read-only, `LIMIT`-capped), return `columns []string` + sample `rows`.
- Compile+validate endpoint wrapping `studio.Generate` + `studio.Validate` (passing the live `DB`/`DBEngine`).
- Preview endpoint wrapping `studio.PreviewField` against a cached sample row.
- Static file serving for the UI; export-YAML endpoint.

**Plan 3 — Web wizard UI** (served by Plan 2):
- The resource-type-as-unit flow validated in the mockup: connect → declare resource types → per-type List/Entitlements/Grants with column-pick + recipe chips + raw-CEL-with-preview → review + export.
- Consumes Plan 2's endpoints; renders live results, the generated-YAML drawer, and inline validation issues from the `Report`.

---

## Execution Handoff

Plan complete. Two execution options:

1. **Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks.
2. **Inline Execution** — execute tasks in this session with checkpoints.
