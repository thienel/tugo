package query

import (
	"fmt"
	"strings"

	"github.com/thienel/tugo/pkg/apperror"
)

// SortDirection represents sort order.
type SortDirection string

const (
	SortAsc  SortDirection = "ASC"
	SortDesc SortDirection = "DESC"
)

// Sort represents a single sort specification.
type Sort struct {
	Field     string
	Direction SortDirection
}

// SortParser parses sort query parameters.
type SortParser struct {
	allowedFields map[string]bool
}

// NewSortParser creates a new sort parser.
func NewSortParser(allowedFields []string) *SortParser {
	fieldMap := make(map[string]bool)
	for _, f := range allowedFields {
		fieldMap[f] = true
	}
	return &SortParser{allowedFields: fieldMap}
}

// Parse parses sort parameter.
// Expected format: ?sort=-created_at,name (- prefix for DESC)
func (p *SortParser) Parse(sortParam string) ([]Sort, error) {
	if sortParam == "" {
		return nil, nil
	}

	parts := strings.Split(sortParam, ",")
	sorts := make([]Sort, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		direction := SortAsc
		field := part

		if strings.HasPrefix(part, "-") {
			direction = SortDesc
			field = part[1:]
		} else if strings.HasPrefix(part, "+") {
			field = part[1:]
		}

		// Validate field name
		if sanitizeIdentifier(field) == "" {
			return nil, apperror.ErrInvalidSort.WithMessagef("Invalid field name '%s'", field)
		}

		// Validate against allowed fields
		if len(p.allowedFields) > 0 && !p.allowedFields[field] {
			return nil, apperror.ErrInvalidSort.WithMessagef("Field '%s' is not allowed for sorting", field)
		}

		sorts = append(sorts, Sort{
			Field:     field,
			Direction: direction,
		})
	}

	return sorts, nil
}

// SortsToSQL converts sorts to SQL ORDER BY clause.
func SortsToSQL(sorts []Sort) string {
	if len(sorts) == 0 {
		return ""
	}

	parts := make([]string, len(sorts))
	for i, s := range sorts {
		field := sanitizeIdentifier(s.Field)
		if field == "" {
			continue
		}
		parts[i] = fmt.Sprintf("%s %s", field, s.Direction)
	}

	return strings.Join(parts, ", ")
}

// DefaultSort returns a default sort if none specified.
func DefaultSort(primaryKey string) []Sort {
	if primaryKey == "" {
		return nil
	}
	return []Sort{
		{Field: primaryKey, Direction: SortDesc},
	}
}
