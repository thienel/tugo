package query

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/thienel/tugo/pkg/apperror"
)

// FilterOperator represents a filter comparison operator.
type FilterOperator string

const (
	OpEqual        FilterOperator = "eq"
	OpNotEqual     FilterOperator = "ne"
	OpGreaterThan  FilterOperator = "gt"
	OpGreaterEqual FilterOperator = "gte"
	OpLessThan     FilterOperator = "lt"
	OpLessEqual    FilterOperator = "lte"
	OpLike         FilterOperator = "like"
	OpIn           FilterOperator = "in"
	OpIsNull       FilterOperator = "null"
	OpIsNotNull    FilterOperator = "notnull"
)

// operatorSQL maps operators to SQL operators.
var operatorSQL = map[FilterOperator]string{
	OpEqual:        "=",
	OpNotEqual:     "!=",
	OpGreaterThan:  ">",
	OpGreaterEqual: ">=",
	OpLessThan:     "<",
	OpLessEqual:    "<=",
	OpLike:         "ILIKE",
	OpIn:           "IN",
	OpIsNull:       "IS NULL",
	OpIsNotNull:    "IS NOT NULL",
}

// Filter represents a single filter condition.
type Filter struct {
	Field    string
	Operator FilterOperator
	Value    interface{}
}

// FilterParser parses filter query parameters.
type FilterParser struct {
	allowedFields map[string]bool
}

// NewFilterParser creates a new filter parser.
func NewFilterParser(allowedFields []string) *FilterParser {
	fieldMap := make(map[string]bool)
	for _, f := range allowedFields {
		fieldMap[f] = true
	}
	return &FilterParser{allowedFields: fieldMap}
}

// Parse parses filter parameters from query string.
// Expected format: filter[field]=value or filter[field:op]=value
func (p *FilterParser) Parse(params map[string][]string) ([]Filter, error) {
	filters := make([]Filter, 0)
	filterRegex := regexp.MustCompile(`^filter\[([a-zA-Z_][a-zA-Z0-9_]*)(?::([a-z]+))?\]$`)

	for key, values := range params {
		matches := filterRegex.FindStringSubmatch(key)
		if matches == nil {
			continue
		}

		field := matches[1]
		opStr := matches[2]
		if opStr == "" {
			opStr = "eq"
		}

		// Validate field if allowedFields is set
		if len(p.allowedFields) > 0 && !p.allowedFields[field] {
			return nil, apperror.ErrInvalidFilter.WithMessagef("Field '%s' is not allowed for filtering", field)
		}

		op := FilterOperator(opStr)
		if _, ok := operatorSQL[op]; !ok {
			return nil, apperror.ErrInvalidFilter.WithMessagef("Unknown operator '%s'", opStr)
		}

		value := values[0]
		filters = append(filters, Filter{
			Field:    field,
			Operator: op,
			Value:    value,
		})
	}

	return filters, nil
}

// ToSQL converts filters to SQL WHERE conditions.
func FiltersToSQL(filters []Filter, startParam int) (string, []interface{}) {
	if len(filters) == 0 {
		return "", nil
	}

	conditions := make([]string, 0, len(filters))
	args := make([]interface{}, 0, len(filters))
	paramNum := startParam

	for _, f := range filters {
		condition, filterArgs := filterToSQL(f, paramNum)
		conditions = append(conditions, condition)
		args = append(args, filterArgs...)
		paramNum += len(filterArgs)
	}

	return strings.Join(conditions, " AND "), args
}

// filterToSQL converts a single filter to SQL.
func filterToSQL(f Filter, paramNum int) (string, []interface{}) {
	field := sanitizeIdentifier(f.Field)

	switch f.Operator {
	case OpIsNull:
		return fmt.Sprintf("%s IS NULL", field), nil

	case OpIsNotNull:
		return fmt.Sprintf("%s IS NOT NULL", field), nil

	case OpLike:
		return fmt.Sprintf("%s ILIKE $%d", field, paramNum), []interface{}{"%" + f.Value.(string) + "%"}

	case OpIn:
		values := strings.Split(f.Value.(string), ",")
		placeholders := make([]string, len(values))
		args := make([]interface{}, len(values))
		for i, v := range values {
			placeholders[i] = fmt.Sprintf("$%d", paramNum+i)
			args[i] = strings.TrimSpace(v)
		}
		return fmt.Sprintf("%s IN (%s)", field, strings.Join(placeholders, ", ")), args

	default:
		sqlOp := operatorSQL[f.Operator]
		return fmt.Sprintf("%s %s $%d", field, sqlOp, paramNum), []interface{}{f.Value}
	}
}

// sanitizeIdentifier ensures a field name is safe for SQL.
func sanitizeIdentifier(name string) string {
	// Only allow alphanumeric and underscore
	re := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	if !re.MatchString(name) {
		return ""
	}
	return name
}
