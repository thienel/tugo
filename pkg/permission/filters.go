package permission

import (
	"fmt"
	"strings"
)

// FilterBuilder builds SQL WHERE clauses from permission filters.
type FilterBuilder struct {
	paramOffset int
}

// NewFilterBuilder creates a new filter builder.
func NewFilterBuilder(paramOffset int) *FilterBuilder {
	return &FilterBuilder{
		paramOffset: paramOffset,
	}
}

// Build converts a permission filter to SQL WHERE clause.
func (fb *FilterBuilder) Build(filter map[string]any) (string, []any) {
	if len(filter) == 0 {
		return "", nil
	}

	conditions, args := fb.buildConditions(filter)
	if len(conditions) == 0 {
		return "", nil
	}

	return strings.Join(conditions, " AND "), args
}

// buildConditions recursively builds conditions from filter map.
func (fb *FilterBuilder) buildConditions(filter map[string]any) ([]string, []any) {
	var conditions []string
	var args []any

	for field, value := range filter {
		// Handle special operators at root level
		switch field {
		case "_and":
			if andFilters, ok := value.([]any); ok {
				andConditions, andArgs := fb.buildAndConditions(andFilters)
				if andConditions != "" {
					conditions = append(conditions, andConditions)
					args = append(args, andArgs...)
				}
			}
		case "_or":
			if orFilters, ok := value.([]any); ok {
				orConditions, orArgs := fb.buildOrConditions(orFilters)
				if orConditions != "" {
					conditions = append(conditions, orConditions)
					args = append(args, orArgs...)
				}
			}
		default:
			// Regular field filter
			cond, fieldArgs := fb.buildFieldCondition(field, value)
			if cond != "" {
				conditions = append(conditions, cond)
				args = append(args, fieldArgs...)
			}
		}
	}

	return conditions, args
}

// buildAndConditions builds AND conditions.
func (fb *FilterBuilder) buildAndConditions(filters []any) (string, []any) {
	var conditions []string
	var args []any

	for _, f := range filters {
		if filterMap, ok := f.(map[string]any); ok {
			subConditions, subArgs := fb.buildConditions(filterMap)
			conditions = append(conditions, subConditions...)
			args = append(args, subArgs...)
		}
	}

	if len(conditions) == 0 {
		return "", nil
	}

	return "(" + strings.Join(conditions, " AND ") + ")", args
}

// buildOrConditions builds OR conditions.
func (fb *FilterBuilder) buildOrConditions(filters []any) (string, []any) {
	var conditions []string
	var args []any

	for _, f := range filters {
		if filterMap, ok := f.(map[string]any); ok {
			subConditions, subArgs := fb.buildConditions(filterMap)
			if len(subConditions) > 0 {
				combined := "(" + strings.Join(subConditions, " AND ") + ")"
				conditions = append(conditions, combined)
				args = append(args, subArgs...)
			}
		}
	}

	if len(conditions) == 0 {
		return "", nil
	}

	return "(" + strings.Join(conditions, " OR ") + ")", args
}

// buildFieldCondition builds a condition for a single field.
func (fb *FilterBuilder) buildFieldCondition(field string, value any) (string, []any) {
	// If value is a map, it contains operator specifications
	if opMap, ok := value.(map[string]any); ok {
		return fb.buildOperatorCondition(field, opMap)
	}

	// Default to equality
	fb.paramOffset++
	return fmt.Sprintf("%s = $%d", sanitizeIdentifier(field), fb.paramOffset), []any{value}
}

// buildOperatorCondition builds a condition with operators.
func (fb *FilterBuilder) buildOperatorCondition(field string, ops map[string]any) (string, []any) {
	var conditions []string
	var args []any
	sanitizedField := sanitizeIdentifier(field)

	for op, value := range ops {
		switch op {
		case "_eq":
			fb.paramOffset++
			conditions = append(conditions, fmt.Sprintf("%s = $%d", sanitizedField, fb.paramOffset))
			args = append(args, value)
		case "_ne", "_neq":
			fb.paramOffset++
			conditions = append(conditions, fmt.Sprintf("%s != $%d", sanitizedField, fb.paramOffset))
			args = append(args, value)
		case "_gt":
			fb.paramOffset++
			conditions = append(conditions, fmt.Sprintf("%s > $%d", sanitizedField, fb.paramOffset))
			args = append(args, value)
		case "_gte":
			fb.paramOffset++
			conditions = append(conditions, fmt.Sprintf("%s >= $%d", sanitizedField, fb.paramOffset))
			args = append(args, value)
		case "_lt":
			fb.paramOffset++
			conditions = append(conditions, fmt.Sprintf("%s < $%d", sanitizedField, fb.paramOffset))
			args = append(args, value)
		case "_lte":
			fb.paramOffset++
			conditions = append(conditions, fmt.Sprintf("%s <= $%d", sanitizedField, fb.paramOffset))
			args = append(args, value)
		case "_in":
			if arr, ok := value.([]any); ok && len(arr) > 0 {
				placeholders := make([]string, len(arr))
				for i, v := range arr {
					fb.paramOffset++
					placeholders[i] = fmt.Sprintf("$%d", fb.paramOffset)
					args = append(args, v)
				}
				conditions = append(conditions, fmt.Sprintf("%s IN (%s)", sanitizedField, strings.Join(placeholders, ", ")))
			}
		case "_nin", "_not_in":
			if arr, ok := value.([]any); ok && len(arr) > 0 {
				placeholders := make([]string, len(arr))
				for i, v := range arr {
					fb.paramOffset++
					placeholders[i] = fmt.Sprintf("$%d", fb.paramOffset)
					args = append(args, v)
				}
				conditions = append(conditions, fmt.Sprintf("%s NOT IN (%s)", sanitizedField, strings.Join(placeholders, ", ")))
			}
		case "_like", "_contains":
			fb.paramOffset++
			conditions = append(conditions, fmt.Sprintf("%s ILIKE $%d", sanitizedField, fb.paramOffset))
			args = append(args, "%"+fmt.Sprint(value)+"%")
		case "_nlike", "_not_contains":
			fb.paramOffset++
			conditions = append(conditions, fmt.Sprintf("%s NOT ILIKE $%d", sanitizedField, fb.paramOffset))
			args = append(args, "%"+fmt.Sprint(value)+"%")
		case "_starts_with":
			fb.paramOffset++
			conditions = append(conditions, fmt.Sprintf("%s ILIKE $%d", sanitizedField, fb.paramOffset))
			args = append(args, fmt.Sprint(value)+"%")
		case "_ends_with":
			fb.paramOffset++
			conditions = append(conditions, fmt.Sprintf("%s ILIKE $%d", sanitizedField, fb.paramOffset))
			args = append(args, "%"+fmt.Sprint(value))
		case "_null", "_is_null":
			if boolVal, ok := value.(bool); ok {
				if boolVal {
					conditions = append(conditions, fmt.Sprintf("%s IS NULL", sanitizedField))
				} else {
					conditions = append(conditions, fmt.Sprintf("%s IS NOT NULL", sanitizedField))
				}
			}
		case "_nnull", "_is_not_null":
			conditions = append(conditions, fmt.Sprintf("%s IS NOT NULL", sanitizedField))
		case "_between":
			if arr, ok := value.([]any); ok && len(arr) == 2 {
				fb.paramOffset++
				lowParam := fb.paramOffset
				fb.paramOffset++
				highParam := fb.paramOffset
				conditions = append(conditions, fmt.Sprintf("%s BETWEEN $%d AND $%d", sanitizedField, lowParam, highParam))
				args = append(args, arr[0], arr[1])
			}
		case "_regex":
			fb.paramOffset++
			conditions = append(conditions, fmt.Sprintf("%s ~ $%d", sanitizedField, fb.paramOffset))
			args = append(args, value)
		case "_iregex":
			fb.paramOffset++
			conditions = append(conditions, fmt.Sprintf("%s ~* $%d", sanitizedField, fb.paramOffset))
			args = append(args, value)
		}
	}

	if len(conditions) == 0 {
		return "", nil
	}

	return strings.Join(conditions, " AND "), args
}

// GetParamOffset returns the current parameter offset.
func (fb *FilterBuilder) GetParamOffset() int {
	return fb.paramOffset
}

// sanitizeIdentifier sanitizes a SQL identifier to prevent injection.
func sanitizeIdentifier(name string) string {
	// Only allow alphanumeric and underscore
	var result strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			result.WriteRune(c)
		}
	}
	return result.String()
}

// ApplyPermissionFilter adds permission filter to existing filters.
func ApplyPermissionFilter(existingFilters map[string]any, permFilter map[string]any) map[string]any {
	if permFilter == nil || len(permFilter) == 0 {
		return existingFilters
	}

	if existingFilters == nil || len(existingFilters) == 0 {
		return permFilter
	}

	// Combine with AND
	return map[string]any{
		"_and": []any{existingFilters, permFilter},
	}
}

// MergeFilters merges multiple filter maps.
func MergeFilters(filters ...map[string]any) map[string]any {
	result := make(map[string]any)

	for _, filter := range filters {
		if filter == nil {
			continue
		}
		for k, v := range filter {
			if existing, ok := result[k]; ok {
				// If key exists, create _and condition
				result["_and"] = []any{
					map[string]any{k: existing},
					map[string]any{k: v},
				}
				delete(result, k)
			} else {
				result[k] = v
			}
		}
	}

	return result
}
