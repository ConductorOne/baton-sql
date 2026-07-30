package studio

import (
	"fmt"
	"strings"
)

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
	default:
		return "", fmt.Errorf("unknown or non-simple recipe %q", t.Recipe)
	}
}

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
