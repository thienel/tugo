package query

import (
	"fmt"
	"strconv"
	"strings"
)

// Pagination holds pagination parameters.
type Pagination struct {
	Page   int
	Limit  int
	Offset int
}

// DefaultPagination returns default pagination.
func DefaultPagination() Pagination {
	return Pagination{
		Page:   1,
		Limit:  20,
		Offset: 0,
	}
}

// ParsePagination parses page and limit from query params.
func ParsePagination(params map[string][]string) Pagination {
	p := DefaultPagination()

	if pageStr, ok := params["page"]; ok && len(pageStr) > 0 {
		if page, err := strconv.Atoi(pageStr[0]); err == nil && page > 0 {
			p.Page = page
		}
	}

	if limitStr, ok := params["limit"]; ok && len(limitStr) > 0 {
		if limit, err := strconv.Atoi(limitStr[0]); err == nil && limit > 0 {
			// Cap at 100 to prevent abuse
			if limit > 100 {
				limit = 100
			}
			p.Limit = limit
		}
	}

	p.Offset = (p.Page - 1) * p.Limit
	return p
}

// Builder constructs SQL queries dynamically.
type Builder struct {
	tableName   string
	selectCols  []string
	filters     []Filter
	sorts       []Sort
	pagination  Pagination
	args        []interface{}
	paramOffset int
}

// NewBuilder creates a new query builder.
func NewBuilder(tableName string) *Builder {
	return &Builder{
		tableName:   tableName,
		selectCols:  []string{"*"},
		pagination:  DefaultPagination(),
		paramOffset: 1,
	}
}

// Select sets the columns to select.
func (b *Builder) Select(cols ...string) *Builder {
	if len(cols) > 0 {
		b.selectCols = cols
	}
	return b
}

// Where adds filter conditions.
func (b *Builder) Where(filters []Filter) *Builder {
	b.filters = filters
	return b
}

// OrderBy sets sort specifications.
func (b *Builder) OrderBy(sorts []Sort) *Builder {
	b.sorts = sorts
	return b
}

// Paginate sets pagination.
func (b *Builder) Paginate(p Pagination) *Builder {
	b.pagination = p
	return b
}

// BuildSelect builds a SELECT query.
func (b *Builder) BuildSelect() (string, []interface{}) {
	var sb strings.Builder
	args := make([]interface{}, 0)

	// SELECT clause
	sb.WriteString("SELECT ")
	sb.WriteString(strings.Join(b.selectCols, ", "))

	// FROM clause
	sb.WriteString(" FROM ")
	sb.WriteString(b.tableName)

	// WHERE clause
	if len(b.filters) > 0 {
		whereSQL, whereArgs := FiltersToSQL(b.filters, b.paramOffset)
		if whereSQL != "" {
			sb.WriteString(" WHERE ")
			sb.WriteString(whereSQL)
			args = append(args, whereArgs...)
			b.paramOffset += len(whereArgs)
		}
	}

	// ORDER BY clause
	if len(b.sorts) > 0 {
		orderSQL := SortsToSQL(b.sorts)
		if orderSQL != "" {
			sb.WriteString(" ORDER BY ")
			sb.WriteString(orderSQL)
		}
	}

	// LIMIT and OFFSET
	sb.WriteString(fmt.Sprintf(" LIMIT %d OFFSET %d", b.pagination.Limit, b.pagination.Offset))

	return sb.String(), args
}

// BuildCount builds a COUNT query.
func (b *Builder) BuildCount() (string, []interface{}) {
	var sb strings.Builder
	args := make([]interface{}, 0)

	sb.WriteString("SELECT COUNT(*) FROM ")
	sb.WriteString(b.tableName)

	if len(b.filters) > 0 {
		whereSQL, whereArgs := FiltersToSQL(b.filters, 1)
		if whereSQL != "" {
			sb.WriteString(" WHERE ")
			sb.WriteString(whereSQL)
			args = append(args, whereArgs...)
		}
	}

	return sb.String(), args
}

// BuildSelectByID builds a SELECT query for a single row by ID.
func (b *Builder) BuildSelectByID(idColumn string) (string, []interface{}) {
	var sb strings.Builder

	sb.WriteString("SELECT ")
	sb.WriteString(strings.Join(b.selectCols, ", "))
	sb.WriteString(" FROM ")
	sb.WriteString(b.tableName)
	sb.WriteString(" WHERE ")
	sb.WriteString(idColumn)
	sb.WriteString(" = $1")

	return sb.String(), nil
}

// BuildInsert builds an INSERT query.
func BuildInsert(tableName string, data map[string]interface{}) (string, []interface{}) {
	columns := make([]string, 0, len(data))
	placeholders := make([]string, 0, len(data))
	args := make([]interface{}, 0, len(data))
	i := 1

	for col, val := range data {
		if sanitizeIdentifier(col) == "" {
			continue
		}
		columns = append(columns, col)
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
		args = append(args, val)
		i++
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) RETURNING *",
		tableName,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	return query, args
}

// BuildUpdate builds an UPDATE query.
func BuildUpdate(tableName string, idColumn string, id interface{}, data map[string]interface{}) (string, []interface{}) {
	setClauses := make([]string, 0, len(data))
	args := make([]interface{}, 0, len(data)+1)
	i := 1

	for col, val := range data {
		if sanitizeIdentifier(col) == "" {
			continue
		}
		// Skip ID column
		if col == idColumn {
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, i))
		args = append(args, val)
		i++
	}

	// Add ID as last parameter
	args = append(args, id)

	query := fmt.Sprintf(
		"UPDATE %s SET %s WHERE %s = $%d RETURNING *",
		tableName,
		strings.Join(setClauses, ", "),
		idColumn,
		i,
	)

	return query, args
}

// BuildDelete builds a DELETE query.
func BuildDelete(tableName string, idColumn string) string {
	return fmt.Sprintf("DELETE FROM %s WHERE %s = $1", tableName, idColumn)
}

// ParseExpand parses the expand query parameter.
func ParseExpand(params map[string][]string) []string {
	if expandStr, ok := params["expand"]; ok && len(expandStr) > 0 {
		parts := strings.Split(expandStr[0], ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				result = append(result, p)
			}
		}
		return result
	}
	return nil
}
