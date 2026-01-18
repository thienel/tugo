package validation

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// UniqueChecker is an interface for checking uniqueness.
type UniqueChecker interface {
	IsUnique(ctx context.Context, table, column string, value interface{}, excludeID interface{}) (bool, error)
}

// DBUniqueChecker implements UniqueChecker using sqlx.
type DBUniqueChecker struct {
	db        *sqlx.DB
	idColumn  string
}

// NewDBUniqueChecker creates a new database unique checker.
func NewDBUniqueChecker(db *sqlx.DB, idColumn string) *DBUniqueChecker {
	if idColumn == "" {
		idColumn = "id"
	}
	return &DBUniqueChecker{
		db:       db,
		idColumn: idColumn,
	}
}

// IsUnique checks if a value is unique in the database.
func (c *DBUniqueChecker) IsUnique(ctx context.Context, table, column string, value interface{}, excludeID interface{}) (bool, error) {
	var count int
	var query string
	var args []interface{}

	if excludeID != nil {
		query = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = $1 AND %s != $2", table, column, c.idColumn)
		args = []interface{}{value, excludeID}
	} else {
		query = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = $1", table, column)
		args = []interface{}{value}
	}

	err := c.db.GetContext(ctx, &count, query, args...)
	if err != nil {
		return false, err
	}

	return count == 0, nil
}

// Unique validates that a value is unique in the database.
type Unique struct {
	checker   UniqueChecker
	table     string
	column    string
	excludeID interface{}
}

func (u *Unique) Name() string { return "unique" }

func (u *Unique) Validate(ctx context.Context, value interface{}) error {
	if value == nil {
		return nil
	}

	// Skip empty strings
	if str, ok := value.(string); ok && str == "" {
		return nil
	}

	isUnique, err := u.checker.IsUnique(ctx, u.table, u.column, value, u.excludeID)
	if err != nil {
		return fmt.Errorf("failed to check uniqueness: %w", err)
	}

	if !isUnique {
		return fmt.Errorf("value already exists")
	}

	return nil
}

// SetExcludeID sets the ID to exclude from uniqueness check (for updates).
func (u *Unique) SetExcludeID(id interface{}) *Unique {
	u.excludeID = id
	return u
}

// NewUnique creates a new Unique validator.
func NewUnique(checker UniqueChecker, table, column string) *Unique {
	return &Unique{
		checker: checker,
		table:   table,
		column:  column,
	}
}

// Exists validates that a referenced value exists in another table.
type Exists struct {
	db     *sqlx.DB
	table  string
	column string
}

func (e *Exists) Name() string { return "exists" }

func (e *Exists) Validate(ctx context.Context, value interface{}) error {
	if value == nil {
		return nil
	}

	// Skip empty strings
	if str, ok := value.(string); ok && str == "" {
		return nil
	}

	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = $1", e.table, e.column)
	err := e.db.GetContext(ctx, &count, query, value)
	if err != nil {
		return fmt.Errorf("failed to check existence: %w", err)
	}

	if count == 0 {
		return fmt.Errorf("referenced record does not exist")
	}

	return nil
}

// NewExists creates a new Exists validator.
func NewExists(db *sqlx.DB, table, column string) *Exists {
	return &Exists{
		db:     db,
		table:  table,
		column: column,
	}
}
