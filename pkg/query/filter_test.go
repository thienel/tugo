package query

import (
	"testing"
)

func TestFilterParser_Parse(t *testing.T) {
	tests := []struct {
		name        string
		params      map[string][]string
		allowed     []string
		wantCount   int
		wantErr     bool
		checkFilter func(filters []Filter) bool
	}{
		{
			name: "simple equality filter",
			params: map[string][]string{
				"filter[name]": {"John"},
			},
			wantCount: 1,
			checkFilter: func(filters []Filter) bool {
				return filters[0].Field == "name" &&
					filters[0].Operator == OpEqual &&
					filters[0].Value == "John"
			},
		},
		{
			name: "explicit eq operator",
			params: map[string][]string{
				"filter[status:eq]": {"active"},
			},
			wantCount: 1,
			checkFilter: func(filters []Filter) bool {
				return filters[0].Field == "status" &&
					filters[0].Operator == OpEqual &&
					filters[0].Value == "active"
			},
		},
		{
			name: "greater than operator",
			params: map[string][]string{
				"filter[price:gt]": {"100"},
			},
			wantCount: 1,
			checkFilter: func(filters []Filter) bool {
				return filters[0].Field == "price" &&
					filters[0].Operator == OpGreaterThan &&
					filters[0].Value == "100"
			},
		},
		{
			name: "greater than or equal operator",
			params: map[string][]string{
				"filter[age:gte]": {"18"},
			},
			wantCount: 1,
			checkFilter: func(filters []Filter) bool {
				return filters[0].Field == "age" &&
					filters[0].Operator == OpGreaterEqual
			},
		},
		{
			name: "less than operator",
			params: map[string][]string{
				"filter[quantity:lt]": {"10"},
			},
			wantCount: 1,
			checkFilter: func(filters []Filter) bool {
				return filters[0].Operator == OpLessThan
			},
		},
		{
			name: "less than or equal operator",
			params: map[string][]string{
				"filter[score:lte]": {"100"},
			},
			wantCount: 1,
			checkFilter: func(filters []Filter) bool {
				return filters[0].Operator == OpLessEqual
			},
		},
		{
			name: "like operator",
			params: map[string][]string{
				"filter[name:like]": {"john"},
			},
			wantCount: 1,
			checkFilter: func(filters []Filter) bool {
				return filters[0].Operator == OpLike
			},
		},
		{
			name: "in operator",
			params: map[string][]string{
				"filter[status:in]": {"active,pending,completed"},
			},
			wantCount: 1,
			checkFilter: func(filters []Filter) bool {
				return filters[0].Operator == OpIn &&
					filters[0].Value == "active,pending,completed"
			},
		},
		{
			name: "unknown operator returns error",
			params: map[string][]string{
				"filter[name:invalid]": {"test"},
			},
			wantErr: true,
		},
		{
			name: "multiple filters",
			params: map[string][]string{
				"filter[name]":      {"John"},
				"filter[status:eq]": {"active"},
			},
			wantCount: 2,
		},
		{
			name: "non-filter params ignored",
			params: map[string][]string{
				"filter[name]": {"John"},
				"page":         {"1"},
				"limit":        {"10"},
			},
			wantCount: 1,
		},
		{
			name:      "empty params",
			params:    map[string][]string{},
			wantCount: 0,
		},
		{
			name: "field validation - allowed field",
			params: map[string][]string{
				"filter[name]": {"John"},
			},
			allowed:   []string{"name", "email"},
			wantCount: 1,
		},
		{
			name: "field validation - disallowed field",
			params: map[string][]string{
				"filter[password]": {"secret"},
			},
			allowed: []string{"name", "email"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewFilterParser(tt.allowed)
			filters, err := parser.Parse(tt.params)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(filters) != tt.wantCount {
				t.Errorf("expected %d filters, got %d", tt.wantCount, len(filters))
				return
			}

			if tt.checkFilter != nil && !tt.checkFilter(filters) {
				t.Errorf("filter check failed")
			}
		})
	}
}

func TestFiltersToSQL(t *testing.T) {
	tests := []struct {
		name       string
		filters    []Filter
		startParam int
		wantSQL    string
		wantArgs   int
	}{
		{
			name:       "empty filters",
			filters:    []Filter{},
			startParam: 1,
			wantSQL:    "",
			wantArgs:   0,
		},
		{
			name: "single equality filter",
			filters: []Filter{
				{Field: "name", Operator: OpEqual, Value: "John"},
			},
			startParam: 1,
			wantSQL:    "name = $1",
			wantArgs:   1,
		},
		{
			name: "is null filter",
			filters: []Filter{
				{Field: "deleted_at", Operator: OpIsNull, Value: "true"},
			},
			startParam: 1,
			wantSQL:    "deleted_at IS NULL",
			wantArgs:   0,
		},
		{
			name: "is not null filter",
			filters: []Filter{
				{Field: "email", Operator: OpIsNotNull, Value: "true"},
			},
			startParam: 1,
			wantSQL:    "email IS NOT NULL",
			wantArgs:   0,
		},
		{
			name: "like filter adds wildcards",
			filters: []Filter{
				{Field: "name", Operator: OpLike, Value: "john"},
			},
			startParam: 1,
			wantSQL:    "name ILIKE $1",
			wantArgs:   1,
		},
		{
			name: "in filter",
			filters: []Filter{
				{Field: "status", Operator: OpIn, Value: "active,pending"},
			},
			startParam: 1,
			wantSQL:    "status IN ($1, $2)",
			wantArgs:   2,
		},
		{
			name: "multiple filters combined with AND",
			filters: []Filter{
				{Field: "status", Operator: OpEqual, Value: "active"},
				{Field: "price", Operator: OpGreaterThan, Value: "100"},
			},
			startParam: 1,
			wantSQL:    "status = $1 AND price > $2",
			wantArgs:   2,
		},
		{
			name: "parameter numbering starts at given value",
			filters: []Filter{
				{Field: "name", Operator: OpEqual, Value: "test"},
			},
			startParam: 5,
			wantSQL:    "name = $5",
			wantArgs:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args := FiltersToSQL(tt.filters, tt.startParam)

			if sql != tt.wantSQL {
				t.Errorf("expected SQL %q, got %q", tt.wantSQL, sql)
			}

			if len(args) != tt.wantArgs {
				t.Errorf("expected %d args, got %d", tt.wantArgs, len(args))
			}
		})
	}
}

func TestSanitizeIdentifier(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"name", "name"},
		{"user_id", "user_id"},
		{"createdAt", "createdAt"},
		{"_private", "_private"},
		{"field123", "field123"},
		{"123field", ""},        // Can't start with number
		{"field-name", ""},      // Hyphens not allowed
		{"field.name", ""},      // Dots not allowed
		{"field;DROP", ""},      // SQL injection attempt
		{"field' OR '1'='1", ""}, // SQL injection attempt
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeIdentifier(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeIdentifier(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
