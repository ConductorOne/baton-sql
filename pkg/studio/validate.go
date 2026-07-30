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
