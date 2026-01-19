package query

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

// Options holds all query options for a list request.
type Options struct {
	// Filters contains the filter conditions
	Filters []Filter

	// Sort contains the sort specifications
	Sort []Sort

	// Pagination contains pagination settings
	Pagination Pagination

	// Fields lists the fields to return (empty means all)
	Fields []string

	// Expand lists relationships to expand (populate)
	Expand []string

	// Search is a global search term
	Search string

	// Deep enables deep filtering on related collections
	Deep map[string][]Filter

	// Aggregate contains aggregation operations
	Aggregate []Aggregation

	// GroupBy lists fields to group by
	GroupBy []string
}

// Aggregation represents an aggregation operation.
type Aggregation struct {
	Field    string `json:"field"`
	Function string `json:"function"` // count, sum, avg, min, max
	Alias    string `json:"alias,omitempty"`
}

// DefaultOptions returns options with sensible defaults.
func DefaultOptions() Options {
	return Options{
		Pagination: DefaultPagination(),
	}
}

// ParseOptions parses query options from URL parameters.
func ParseOptions(params url.Values) Options {
	opts := DefaultOptions()

	// Parse filters using FilterParser
	filterParser := NewFilterParser(nil)
	filters, _ := filterParser.Parse(params)
	opts.Filters = filters

	// Parse sort using SortParser
	sortParser := NewSortParser(nil)
	sorts, _ := sortParser.Parse(params.Get("sort"))
	opts.Sort = sorts

	// Parse pagination
	opts.Pagination = ParsePagination(params)

	// Parse fields
	if fieldsStr := params.Get("fields"); fieldsStr != "" {
		opts.Fields = parseCommaSeparated(fieldsStr)
	}

	// Parse expand
	opts.Expand = ParseExpand(params)

	// Parse search
	opts.Search = params.Get("search")

	// Parse deep filters
	opts.Deep = parseDeepFilters(params)

	// Parse aggregation
	if aggStr := params.Get("aggregate"); aggStr != "" {
		opts.Aggregate = parseAggregation(aggStr)
	}

	// Parse group by
	if groupByStr := params.Get("group_by"); groupByStr != "" {
		opts.GroupBy = parseCommaSeparated(groupByStr)
	}

	return opts
}

// ParseOptionsFromMap parses query options from a map.
func ParseOptionsFromMap(params map[string][]string) Options {
	return ParseOptions(url.Values(params))
}

// ToQueryParams converts options back to URL query parameters.
func (o Options) ToQueryParams() url.Values {
	params := make(url.Values)

	// Add filters
	for _, f := range o.Filters {
		key := "filter[" + f.Field
		if f.Operator != "" && f.Operator != OpEqual {
			key += ":" + string(f.Operator)
		}
		key += "]"
		params.Add(key, formatFilterValue(f.Value))
	}

	// Add sort
	if len(o.Sort) > 0 {
		sortParts := make([]string, len(o.Sort))
		for i, s := range o.Sort {
			if s.Direction == SortDesc {
				sortParts[i] = "-" + s.Field
			} else {
				sortParts[i] = s.Field
			}
		}
		params.Set("sort", strings.Join(sortParts, ","))
	}

	// Add pagination
	if o.Pagination.Page > 1 {
		params.Set("page", strconv.Itoa(o.Pagination.Page))
	}
	if o.Pagination.Limit != 20 {
		params.Set("limit", strconv.Itoa(o.Pagination.Limit))
	}

	// Add fields
	if len(o.Fields) > 0 {
		params.Set("fields", strings.Join(o.Fields, ","))
	}

	// Add expand
	if len(o.Expand) > 0 {
		params.Set("expand", strings.Join(o.Expand, ","))
	}

	// Add search
	if o.Search != "" {
		params.Set("search", o.Search)
	}

	// Add group by
	if len(o.GroupBy) > 0 {
		params.Set("group_by", strings.Join(o.GroupBy, ","))
	}

	return params
}

// WithFilter adds a filter to the options.
func (o Options) WithFilter(field string, operator FilterOperator, value any) Options {
	o.Filters = append(o.Filters, Filter{
		Field:    field,
		Operator: operator,
		Value:    value,
	})
	return o
}

// WithSort adds a sort specification.
func (o Options) WithSort(field string, direction SortDirection) Options {
	o.Sort = append(o.Sort, Sort{
		Field:     field,
		Direction: direction,
	})
	return o
}

// WithPagination sets pagination.
func (o Options) WithPagination(page, limit int) Options {
	o.Pagination = Pagination{
		Page:   page,
		Limit:  limit,
		Offset: (page - 1) * limit,
	}
	return o
}

// WithFields sets the fields to return.
func (o Options) WithFields(fields ...string) Options {
	o.Fields = fields
	return o
}

// WithExpand sets relationships to expand.
func (o Options) WithExpand(expand ...string) Options {
	o.Expand = expand
	return o
}

// WithSearch sets the search term.
func (o Options) WithSearch(search string) Options {
	o.Search = search
	return o
}

// parseCommaSeparated splits a comma-separated string into trimmed parts.
func parseCommaSeparated(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// parseDeepFilters parses deep filter syntax: deep[relation][field][operator]=value
func parseDeepFilters(params url.Values) map[string][]Filter {
	deep := make(map[string][]Filter)

	for key, values := range params {
		if !strings.HasPrefix(key, "deep[") {
			continue
		}

		// Parse: deep[relation][field][operator]
		rest := strings.TrimPrefix(key, "deep[")
		closeBracket := strings.Index(rest, "]")
		if closeBracket == -1 {
			continue
		}

		relation := rest[:closeBracket]
		rest = rest[closeBracket+1:]

		// Parse [field][operator] or [field]
		if !strings.HasPrefix(rest, "[") {
			continue
		}
		rest = rest[1:]

		closeBracket = strings.Index(rest, "]")
		if closeBracket == -1 {
			continue
		}

		field := rest[:closeBracket]
		rest = rest[closeBracket+1:]

		// Check for operator
		operator := OpEqual
		if strings.HasPrefix(rest, "[") && strings.HasSuffix(rest, "]") {
			operator = FilterOperator(rest[1 : len(rest)-1])
		}

		if len(values) > 0 {
			filter := Filter{
				Field:    field,
				Operator: operator,
				Value:    values[0],
			}

			if _, ok := deep[relation]; !ok {
				deep[relation] = make([]Filter, 0)
			}
			deep[relation] = append(deep[relation], filter)
		}
	}

	return deep
}

// parseAggregation parses aggregation parameters.
func parseAggregation(s string) []Aggregation {
	var aggregations []Aggregation

	// Try JSON format first: [{"function":"count","field":"id"}]
	if strings.HasPrefix(s, "[") {
		if err := json.Unmarshal([]byte(s), &aggregations); err == nil {
			return aggregations
		}
	}

	// Simple format: function(field),function(field)
	parts := strings.Split(s, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Parse function(field)
		openParen := strings.Index(part, "(")
		closeParen := strings.Index(part, ")")
		if openParen == -1 || closeParen == -1 || openParen >= closeParen {
			continue
		}

		function := strings.TrimSpace(part[:openParen])
		field := strings.TrimSpace(part[openParen+1 : closeParen])

		aggregations = append(aggregations, Aggregation{
			Function: function,
			Field:    field,
		})
	}

	return aggregations
}

// formatFilterValue converts a filter value to string.
func formatFilterValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case []string:
		return strings.Join(val, ",")
	case []any:
		parts := make([]string, len(val))
		for i, item := range val {
			parts[i] = formatFilterValue(item)
		}
		return strings.Join(parts, ",")
	default:
		return strings.TrimSpace(strings.Trim(strings.Trim(string(mustJSON(val)), "\""), "\""))
	}
}

// mustJSON marshals a value to JSON, returning empty string on error.
func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
