package studio

import (
	"context"
	"database/sql"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/conductorone/baton-sql/pkg/bcel"
	"github.com/conductorone/baton-sql/pkg/bsql"
	"github.com/conductorone/baton-sql/pkg/database"
)

// validPurpose reports whether p is an accepted entitlement purpose. bsql
// only recognizes "assignment" and "permission" (config-driven switch);
// anything else is silently mapped to UNSPECIFIED. Empty is allowed (means
// unspecified intentionally).
func validPurpose(p string) bool {
	switch p {
	case "", "assignment", "permission":
		return true
	default:
		return false
	}
}

// literalPurpose extracts a literal string value from a raw CEL expression
// that is a single quoted string literal (e.g. `'assignment'` or
// `"permission"`). It returns ok=false when the expression is not a bare
// string literal (e.g. a function call or column ref), in which case the
// purpose is evaluated per-row and cannot be checked statically.
func literalPurpose(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if len(s) < 2 {
		return "", false
	}
	q := s[0]
	if (q != '\'' && q != '"') || s[len(s)-1] != q {
		return "", false
	}
	inner := s[1 : len(s)-1]
	// Reject if the inner value itself contains the quote char (i.e. this
	// wasn't a single simple literal).
	if strings.IndexByte(inner, q) >= 0 {
		return "", false
	}
	return inner, true
}

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

// Validate authoritatively validates a Spec by generating its YAML and
// round-tripping it through baton-sql's own parser and syncer validation.
// It layers a spec-level check (principal_type must reference a defined
// resource type, Trap #4) on top of that authoritative pass.
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
			// FIX-2: resource_id is not a supported grant mapping key (bsql's
			// GrantMapping has no such field) — a user who maps it gets a
			// silent no-op, so flag it instead.
			for _, fm := range g.Fields {
				if fm.Field == "resource_id" {
					rep.OK = false
					rep.Errors = append(rep.Errors, Issue{Scope: rt.ID, Field: "resource_id",
						Message: "\"resource_id\" is not a supported grant mapping; remove it (the grant is already scoped to its resource type)"})
				}
			}
		}

		// FIX-1: every grantable_to entry (static per-entitlement and dynamic)
		// must reference a defined resource-type ID; bsql matches these
		// literally and silently drops any that don't resolve.
		for _, e := range rt.Entitlements.Static {
			for _, gt := range e.GrantableTo {
				if !defined[gt] {
					rep.OK = false
					rep.Errors = append(rep.Errors, Issue{Scope: rt.ID, Field: "grantable_to",
						Message: "grantable_to \"" + gt + "\" is not a defined resource type"})
				}
			}
			// FIX-3: purpose must be assignment|permission (or empty); bsql
			// treats anything else as UNSPECIFIED silently.
			if !validPurpose(e.Purpose) {
				rep.OK = false
				rep.Errors = append(rep.Errors, Issue{Scope: rt.ID, Field: "purpose",
					Message: "purpose \"" + e.Purpose + "\" must be \"assignment\" or \"permission\""})
			}
		}
		for _, gt := range rt.Entitlements.GrantableTo {
			if !defined[gt] {
				rep.OK = false
				rep.Errors = append(rep.Errors, Issue{Scope: rt.ID, Field: "grantable_to",
					Message: "grantable_to \"" + gt + "\" is not a defined resource type"})
			}
		}
		// FIX-3: a dynamic entitlement's purpose is authored as a field
		// mapping. Only a literal (raw_cel quoted-string) purpose is
		// statically checkable — a column-sourced purpose is evaluated
		// per-row and its value can't be known here.
		for _, fm := range rt.Entitlements.Fields {
			if fm.Field != "purpose" || fm.Transform == nil {
				continue
			}
			if p, ok := literalPurpose(fm.Transform.RawCEL); ok && !validPurpose(p) {
				rep.OK = false
				rep.Errors = append(rep.Errors, Issue{Scope: rt.ID, Field: "purpose",
					Message: "purpose \"" + p + "\" must be \"assignment\" or \"permission\""})
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
