package query

import (
	"testing"
)

func TestSortParser_Parse(t *testing.T) {
	tests := []struct {
		name      string
		sortParam string
		allowed   []string
		wantCount int
		wantErr   bool
		checkSort func(sorts []Sort) bool
	}{
		{
			name:      "empty sort parameter",
			sortParam: "",
			wantCount: 0,
		},
		{
			name:      "single field ascending",
			sortParam: "name",
			wantCount: 1,
			checkSort: func(sorts []Sort) bool {
				return sorts[0].Field == "name" && sorts[0].Direction == SortAsc
			},
		},
		{
			name:      "single field descending",
			sortParam: "-created_at",
			wantCount: 1,
			checkSort: func(sorts []Sort) bool {
				return sorts[0].Field == "created_at" && sorts[0].Direction == SortDesc
			},
		},
		{
			name:      "explicit ascending with plus",
			sortParam: "+name",
			wantCount: 1,
			checkSort: func(sorts []Sort) bool {
				return sorts[0].Field == "name" && sorts[0].Direction == SortAsc
			},
		},
		{
			name:      "multiple fields",
			sortParam: "-created_at,name",
			wantCount: 2,
			checkSort: func(sorts []Sort) bool {
				return sorts[0].Field == "created_at" && sorts[0].Direction == SortDesc &&
					sorts[1].Field == "name" && sorts[1].Direction == SortAsc
			},
		},
		{
			name:      "multiple fields with mixed directions",
			sortParam: "-price,+name,-id",
			wantCount: 3,
			checkSort: func(sorts []Sort) bool {
				return sorts[0].Direction == SortDesc &&
					sorts[1].Direction == SortAsc &&
					sorts[2].Direction == SortDesc
			},
		},
		{
			name:      "whitespace handling",
			sortParam: " -name , status ",
			wantCount: 2,
		},
		{
			name:      "allowed field validation passes",
			sortParam: "name",
			allowed:   []string{"name", "email"},
			wantCount: 1,
		},
		{
			name:      "disallowed field validation fails",
			sortParam: "password",
			allowed:   []string{"name", "email"},
			wantErr:   true,
		},
		{
			name:      "invalid field name",
			sortParam: "field;DROP TABLE",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewSortParser(tt.allowed)
			sorts, err := parser.Parse(tt.sortParam)

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

			if len(sorts) != tt.wantCount {
				t.Errorf("expected %d sorts, got %d", tt.wantCount, len(sorts))
				return
			}

			if tt.checkSort != nil && !tt.checkSort(sorts) {
				t.Errorf("sort check failed")
			}
		})
	}
}

func TestSortsToSQL(t *testing.T) {
	tests := []struct {
		name    string
		sorts   []Sort
		wantSQL string
	}{
		{
			name:    "empty sorts",
			sorts:   []Sort{},
			wantSQL: "",
		},
		{
			name: "single ascending",
			sorts: []Sort{
				{Field: "name", Direction: SortAsc},
			},
			wantSQL: "name ASC",
		},
		{
			name: "single descending",
			sorts: []Sort{
				{Field: "created_at", Direction: SortDesc},
			},
			wantSQL: "created_at DESC",
		},
		{
			name: "multiple sorts",
			sorts: []Sort{
				{Field: "created_at", Direction: SortDesc},
				{Field: "name", Direction: SortAsc},
			},
			wantSQL: "created_at DESC, name ASC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql := SortsToSQL(tt.sorts)
			if sql != tt.wantSQL {
				t.Errorf("expected SQL %q, got %q", tt.wantSQL, sql)
			}
		})
	}
}

func TestDefaultSort(t *testing.T) {
	tests := []struct {
		name       string
		primaryKey string
		wantCount  int
		checkSort  func(sorts []Sort) bool
	}{
		{
			name:       "with primary key",
			primaryKey: "id",
			wantCount:  1,
			checkSort: func(sorts []Sort) bool {
				return sorts[0].Field == "id" && sorts[0].Direction == SortDesc
			},
		},
		{
			name:       "empty primary key",
			primaryKey: "",
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sorts := DefaultSort(tt.primaryKey)

			if len(sorts) != tt.wantCount {
				t.Errorf("expected %d sorts, got %d", tt.wantCount, len(sorts))
				return
			}

			if tt.checkSort != nil && !tt.checkSort(sorts) {
				t.Errorf("sort check failed")
			}
		})
	}
}
