package schema

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// Introspector queries PostgreSQL for schema information.
type Introspector struct {
	db *sqlx.DB
}

// NewIntrospector creates a new Introspector.
func NewIntrospector(db *sqlx.DB) *Introspector {
	return &Introspector{db: db}
}

// GetTables returns all table names matching the given prefix.
func (i *Introspector) GetTables(ctx context.Context, prefix string) ([]string, error) {
	query := `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public'
		AND table_type = 'BASE TABLE'
		AND table_name LIKE $1
		ORDER BY table_name
	`
	var tables []string
	err := i.db.SelectContext(ctx, &tables, query, prefix+"%")
	if err != nil {
		return nil, err
	}
	return tables, nil
}

// GetColumns returns column information for a table.
func (i *Introspector) GetColumns(ctx context.Context, tableName string) ([]PostgresColumnInfo, error) {
	query := `
		SELECT
			table_name,
			column_name,
			data_type,
			udt_name,
			is_nullable,
			column_default,
			character_maximum_length,
			numeric_precision,
			numeric_scale
		FROM information_schema.columns
		WHERE table_schema = 'public'
		AND table_name = $1
		ORDER BY ordinal_position
	`
	var columns []PostgresColumnInfo
	err := i.db.SelectContext(ctx, &columns, query, tableName)
	if err != nil {
		return nil, err
	}
	return columns, nil
}

// GetPrimaryKeys returns primary key columns for a table.
func (i *Introspector) GetPrimaryKeys(ctx context.Context, tableName string) ([]PostgresPrimaryKeyInfo, error) {
	query := `
		SELECT
			tc.table_name,
			kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		WHERE tc.constraint_type = 'PRIMARY KEY'
		AND tc.table_schema = 'public'
		AND tc.table_name = $1
	`
	var pks []PostgresPrimaryKeyInfo
	err := i.db.SelectContext(ctx, &pks, query, tableName)
	if err != nil {
		return nil, err
	}
	return pks, nil
}

// GetForeignKeys returns foreign key information for a table.
func (i *Introspector) GetForeignKeys(ctx context.Context, tableName string) ([]PostgresForeignKeyInfo, error) {
	query := `
		SELECT
			tc.constraint_name,
			tc.table_name,
			kcu.column_name,
			ccu.table_name AS foreign_table_name,
			ccu.column_name AS foreign_column_name,
			rc.delete_rule,
			rc.update_rule
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu
			ON ccu.constraint_name = tc.constraint_name
			AND ccu.table_schema = tc.table_schema
		JOIN information_schema.referential_constraints rc
			ON tc.constraint_name = rc.constraint_name
			AND tc.table_schema = rc.constraint_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
		AND tc.table_schema = 'public'
		AND tc.table_name = $1
	`
	var fks []PostgresForeignKeyInfo
	err := i.db.SelectContext(ctx, &fks, query, tableName)
	if err != nil {
		return nil, err
	}
	return fks, nil
}

// GetUniqueColumns returns columns with unique constraints.
func (i *Introspector) GetUniqueColumns(ctx context.Context, tableName string) ([]PostgresUniqueInfo, error) {
	query := `
		SELECT
			tc.table_name,
			kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		WHERE tc.constraint_type = 'UNIQUE'
		AND tc.table_schema = 'public'
		AND tc.table_name = $1
	`
	var uniques []PostgresUniqueInfo
	err := i.db.SelectContext(ctx, &uniques, query, tableName)
	if err != nil {
		return nil, err
	}
	return uniques, nil
}

// GetAllForeignKeys returns all foreign keys in the database.
func (i *Introspector) GetAllForeignKeys(ctx context.Context, prefix string) ([]PostgresForeignKeyInfo, error) {
	query := `
		SELECT
			tc.constraint_name,
			tc.table_name,
			kcu.column_name,
			ccu.table_name AS foreign_table_name,
			ccu.column_name AS foreign_column_name,
			rc.delete_rule,
			rc.update_rule
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu
			ON ccu.constraint_name = tc.constraint_name
			AND ccu.table_schema = tc.table_schema
		JOIN information_schema.referential_constraints rc
			ON tc.constraint_name = rc.constraint_name
			AND tc.table_schema = rc.constraint_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
		AND tc.table_schema = 'public'
		AND tc.table_name LIKE $1
	`
	var fks []PostgresForeignKeyInfo
	err := i.db.SelectContext(ctx, &fks, query, prefix+"%")
	if err != nil {
		return nil, err
	}
	return fks, nil
}

// TableExists checks if a table exists.
func (i *Introspector) TableExists(ctx context.Context, tableName string) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = $1
		)
	`
	var exists bool
	err := i.db.GetContext(ctx, &exists, query)
	if err != nil {
		return false, err
	}
	return exists, nil
}
