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
