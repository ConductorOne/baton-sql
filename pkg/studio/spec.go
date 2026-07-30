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
	Recipe string         `json:"recipe"` // see recipes.go
	Args   map[string]any `json:"args,omitempty"`
	RawCEL string         `json:"raw_cel,omitempty"`
}

type EntitlementsSpec struct {
	Mode        string              `json:"mode"` // "static" | "query" | "none"
	Static      []StaticEntitlement `json:"static,omitempty"`
	Query       string              `json:"query,omitempty"`
	Fields      []FieldMapping      `json:"fields,omitempty"`       // for query mode
	GrantableTo []string            `json:"grantable_to,omitempty"` // resource-type IDs this entitlement is grantable to (dynamic mode)
}

type StaticEntitlement struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description,omitempty"`
	Purpose     string   `json:"purpose,omitempty"`
	GrantableTo []string `json:"grantable_to,omitempty"` // resource-type IDs this entitlement is grantable to
	Immutable   bool     `json:"immutable,omitempty"`
}

// GrantSpec models one grants query. A single query can fan out to MANY
// principal mappings (e.g. Redshift's grants query producing one row per
// principal_type x entitlement), so Mappings is a list: one entry per row of
// the emitted bsql grant map: sequence.
type GrantSpec struct {
	Query       string         `json:"query"`
	ResourceVar string         `json:"resource_var,omitempty"` // ?<var> bound to resource.ID
	Mappings    []GrantMapping `json:"mappings"`
}

// GrantMapping is one row of a grants query's map: sequence — a single
// principal_id/principal_type/entitlement grant, optionally guarded by skip_if.
type GrantMapping struct {
	PrincipalID   FieldMapping  `json:"principal_id"`          // column/CEL for the principal id
	PrincipalType string        `json:"principal_type"`        // literal resource-type id
	Entitlement   string        `json:"entitlement,omitempty"` // entitlement id/slug this row grants
	SkipIf        *FieldMapping `json:"skip_if,omitempty"`     // optional CEL bool
}
