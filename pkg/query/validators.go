package query

import (
	"fmt"
	"regexp"
	"strings"
)

// FieldValidator validates field names for SQL injection prevention and correctness.
type FieldValidator struct {
	allowedFields   map[string]bool
	reservedWords   map[string]bool
	identifierRegex *regexp.Regexp
}

// NewFieldValidator creates a new field validator with allowed fields.
func NewFieldValidator(allowedFields []string) *FieldValidator {
	allowed := make(map[string]bool)
	for _, f := range allowedFields {
		allowed[strings.ToLower(f)] = true
	}

	return &FieldValidator{
		allowedFields:   allowed,
		reservedWords:   getReservedWords(),
		identifierRegex: regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`),
	}
}

// ValidateField checks if a field name is valid and allowed.
func (v *FieldValidator) ValidateField(field string) error {
	field = strings.ToLower(field)

	// Check if empty
	if field == "" {
		return fmt.Errorf("field name cannot be empty")
	}

	// Check identifier format
	if !v.identifierRegex.MatchString(field) {
		return fmt.Errorf("field '%s' contains invalid characters", field)
	}

	// Check reserved words
	if v.reservedWords[field] {
		return fmt.Errorf("field '%s' is a reserved SQL word", field)
	}

	// Check if in allowed list (if list is provided)
	if len(v.allowedFields) > 0 && !v.allowedFields[field] {
		return fmt.Errorf("field '%s' is not allowed", field)
	}

	return nil
}

// ValidateFields validates multiple field names.
func (v *FieldValidator) ValidateFields(fields []string) error {
	for _, field := range fields {
		if err := v.ValidateField(field); err != nil {
			return err
		}
	}
	return nil
}

// SanitizeField sanitizes a field name, removing dangerous characters.
func (v *FieldValidator) SanitizeField(field string) string {
	return sanitizeIdentifier(field)
}

// SanitizeFields sanitizes multiple field names.
func (v *FieldValidator) SanitizeFields(fields []string) []string {
	result := make([]string, 0, len(fields))
	for _, f := range fields {
		sanitized := v.SanitizeField(f)
		if sanitized != "" {
			result = append(result, sanitized)
		}
	}
	return result
}

// FilterValidator validates filter operations.
type FilterValidator struct {
	fieldValidator *FieldValidator
}

// NewFilterValidator creates a new filter validator.
func NewFilterValidator(allowedFields []string) *FilterValidator {
	return &FilterValidator{
		fieldValidator: NewFieldValidator(allowedFields),
	}
}

// ValidateFilter validates a single filter.
func (v *FilterValidator) ValidateFilter(f Filter) error {
	// Validate field name
	if err := v.fieldValidator.ValidateField(f.Field); err != nil {
		return err
	}

	// Validate operator
	if !isValidFilterOperator(f.Operator) {
		return fmt.Errorf("invalid operator '%s'", f.Operator)
	}

	// Validate value based on operator
	if err := validateFilterValue(f.Operator, f.Value); err != nil {
		return fmt.Errorf("invalid value for filter '%s': %w", f.Field, err)
	}

	return nil
}

// ValidateFilters validates multiple filters.
func (v *FilterValidator) ValidateFilters(filters []Filter) error {
	for _, f := range filters {
		if err := v.ValidateFilter(f); err != nil {
			return err
		}
	}
	return nil
}

// SortValidator validates sort operations.
type SortValidator struct {
	fieldValidator *FieldValidator
}

// NewSortValidator creates a new sort validator.
func NewSortValidator(allowedFields []string) *SortValidator {
	return &SortValidator{
		fieldValidator: NewFieldValidator(allowedFields),
	}
}

// ValidateSort validates a single sort specification.
func (v *SortValidator) ValidateSort(s Sort) error {
	return v.fieldValidator.ValidateField(s.Field)
}

// ValidateSorts validates multiple sort specifications.
func (v *SortValidator) ValidateSorts(sorts []Sort) error {
	for _, s := range sorts {
		if err := v.ValidateSort(s); err != nil {
			return err
		}
	}
	return nil
}

// isValidFilterOperator checks if a filter operator is valid.
func isValidFilterOperator(op FilterOperator) bool {
	validOps := map[FilterOperator]bool{
		"":          true, // empty means eq
		OpEqual:     true,
		OpNotEqual:  true,
		OpGreaterThan:  true,
		OpGreaterEqual: true,
		OpLessThan:     true,
		OpLessEqual:    true,
		OpIn:           true,
		OpLike:         true,
		OpIsNull:       true,
		OpIsNotNull:    true,
	}
	return validOps[op]
}

// validateFilterValue validates a filter value based on operator.
func validateFilterValue(operator FilterOperator, value any) error {
	switch operator {
	case OpIn:
		// Value should be an array or comma-separated string
		switch v := value.(type) {
		case []any:
			if len(v) == 0 {
				return fmt.Errorf("'in' operator requires at least one value")
			}
		case []string:
			if len(v) == 0 {
				return fmt.Errorf("'in' operator requires at least one value")
			}
		case string:
			if v == "" {
				return fmt.Errorf("'in' operator requires at least one value")
			}
		default:
			return fmt.Errorf("'in' operator requires array value")
		}

	case OpIsNull, OpIsNotNull:
		// These operators typically don't need a value or take a boolean
		switch v := value.(type) {
		case bool:
			// OK
		case string:
			if v != "" && v != "true" && v != "false" {
				return fmt.Errorf("'null' operator requires boolean value")
			}
		case nil:
			// OK
		default:
			return fmt.Errorf("'null' operator requires boolean value")
		}
	}

	return nil
}

// getReservedWords returns a map of SQL reserved words.
func getReservedWords() map[string]bool {
	return map[string]bool{
		"select": true, "from": true, "where": true, "insert": true,
		"update": true, "delete": true, "drop": true, "create": true,
		"alter": true, "table": true, "index": true, "into": true,
		"values": true, "set": true, "and": true, "or": true,
		"not": true, "null": true, "is": true, "like": true,
		"in": true, "between": true, "join": true, "on": true,
		"left": true, "right": true, "inner": true, "outer": true,
		"group": true, "by": true, "order": true, "asc": true,
		"desc": true, "limit": true, "offset": true, "having": true,
		"union": true, "all": true, "distinct": true, "as": true,
		"case": true, "when": true, "then": true, "else": true,
		"end": true, "true": true, "false": true, "exists": true,
		"execute": true, "grant": true, "revoke": true, "truncate": true,
	}
}

// OptionsValidator validates complete query options.
type OptionsValidator struct {
	fieldValidator  *FieldValidator
	filterValidator *FilterValidator
	sortValidator   *SortValidator
}

// NewOptionsValidator creates a new options validator.
func NewOptionsValidator(allowedFields []string) *OptionsValidator {
	return &OptionsValidator{
		fieldValidator:  NewFieldValidator(allowedFields),
		filterValidator: NewFilterValidator(allowedFields),
		sortValidator:   NewSortValidator(allowedFields),
	}
}

// ValidateOptions validates all query options.
func (v *OptionsValidator) ValidateOptions(opts Options) error {
	// Validate filters
	if err := v.filterValidator.ValidateFilters(opts.Filters); err != nil {
		return fmt.Errorf("invalid filter: %w", err)
	}

	// Validate sort
	if err := v.sortValidator.ValidateSorts(opts.Sort); err != nil {
		return fmt.Errorf("invalid sort: %w", err)
	}

	// Validate fields
	for _, field := range opts.Fields {
		if err := v.fieldValidator.ValidateField(field); err != nil {
			return fmt.Errorf("invalid field selection: %w", err)
		}
	}

	// Validate pagination
	if opts.Pagination.Limit < 0 || opts.Pagination.Limit > 1000 {
		return fmt.Errorf("limit must be between 0 and 1000")
	}
	if opts.Pagination.Page < 1 {
		return fmt.Errorf("page must be at least 1")
	}

	// Validate group by
	for _, field := range opts.GroupBy {
		if err := v.fieldValidator.ValidateField(field); err != nil {
			return fmt.Errorf("invalid group by field: %w", err)
		}
	}

	return nil
}

// ValidateExpand validates expand/relation fields.
func (v *OptionsValidator) ValidateExpand(expand []string, allowedRelations []string) error {
	if len(allowedRelations) == 0 {
		return nil // No restrictions
	}

	allowed := make(map[string]bool)
	for _, r := range allowedRelations {
		allowed[strings.ToLower(r)] = true
	}

	for _, e := range expand {
		// Handle nested expansions like "author.posts"
		parts := strings.Split(e, ".")
		if !allowed[strings.ToLower(parts[0])] {
			return fmt.Errorf("relation '%s' is not allowed for expansion", parts[0])
		}
	}

	return nil
}
