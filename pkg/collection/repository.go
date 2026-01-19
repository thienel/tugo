package collection

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/thienel/tugo/pkg/apperror"
	"github.com/thienel/tugo/pkg/query"
	"github.com/thienel/tugo/pkg/schema"
)

// Repository handles data access for dynamic collections.
type Repository struct {
	db *sqlx.DB
}

// NewRepository creates a new repository.
func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// ListResult contains the results of a list query.
type ListResult struct {
	Items []map[string]any
	Total int
}

// List retrieves items with filtering, sorting, and pagination.
func (r *Repository) List(ctx context.Context, collection *schema.Collection, opts ListOptions) (*ListResult, error) {
	builder := query.NewBuilder(collection.TableName).
		Where(opts.Filters).
		OrderBy(opts.Sorts).
		Paginate(opts.Pagination)

	// Build and execute count query
	countSQL, countArgs := builder.BuildCount()
	var total int
	if err := r.db.GetContext(ctx, &total, countSQL, countArgs...); err != nil {
		return nil, apperror.ErrInternalServer.WithError(err)
	}

	// Build and execute select query
	selectSQL, selectArgs := builder.BuildSelect()
	rows, err := r.db.QueryxContext(ctx, selectSQL, selectArgs...)
	if err != nil {
		return nil, apperror.ErrInternalServer.WithError(err)
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		item := make(map[string]any)
		if err := rows.MapScan(item); err != nil {
			return nil, apperror.ErrInternalServer.WithError(err)
		}
		normalizeMapValues(item)
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, apperror.ErrInternalServer.WithError(err)
	}

	return &ListResult{
		Items: items,
		Total: total,
	}, nil
}

// GetByID retrieves a single item by ID.
func (r *Repository) GetByID(ctx context.Context, collection *schema.Collection, id any) (map[string]any, error) {
	builder := query.NewBuilder(collection.TableName)
	querySQL, _ := builder.BuildSelectByID(collection.PrimaryKey)

	row := r.db.QueryRowxContext(ctx, querySQL, id)
	item := make(map[string]any)
	if err := row.MapScan(item); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperror.ErrNotFound.WithMessagef("Item with ID '%v' not found", id)
		}
		return nil, apperror.ErrInternalServer.WithError(err)
	}

	normalizeMapValues(item)
	return item, nil
}

// Create inserts a new item.
func (r *Repository) Create(ctx context.Context, collection *schema.Collection, data map[string]any) (map[string]any, error) {
	querySQL, args := query.BuildInsert(collection.TableName, data)

	row := r.db.QueryRowxContext(ctx, querySQL, args...)
	result := make(map[string]any)
	if err := row.MapScan(result); err != nil {
		if isDuplicateKeyError(err) {
			return nil, apperror.ErrConflict.WithMessage("Record already exists")
		}
		return nil, apperror.ErrInternalServer.WithError(err)
	}

	normalizeMapValues(result)
	return result, nil
}

// Update updates an existing item.
func (r *Repository) Update(ctx context.Context, collection *schema.Collection, id any, data map[string]any) (map[string]any, error) {
	// Check if item exists
	_, err := r.GetByID(ctx, collection, id)
	if err != nil {
		return nil, err
	}

	querySQL, args := query.BuildUpdate(collection.TableName, collection.PrimaryKey, id, data)

	row := r.db.QueryRowxContext(ctx, querySQL, args...)
	result := make(map[string]any)
	if err := row.MapScan(result); err != nil {
		if isDuplicateKeyError(err) {
			return nil, apperror.ErrConflict.WithMessage("Record with this value already exists")
		}
		return nil, apperror.ErrInternalServer.WithError(err)
	}

	normalizeMapValues(result)
	return result, nil
}

// Delete removes an item by ID.
func (r *Repository) Delete(ctx context.Context, collection *schema.Collection, id any) error {
	// Check if item exists
	_, err := r.GetByID(ctx, collection, id)
	if err != nil {
		return err
	}

	querySQL := query.BuildDelete(collection.TableName, collection.PrimaryKey)
	_, err = r.db.ExecContext(ctx, querySQL, id)
	if err != nil {
		return apperror.ErrInternalServer.WithError(err)
	}

	return nil
}

// GetRelated retrieves related items for expansion.
func (r *Repository) GetRelated(ctx context.Context, relatedCollection *schema.Collection, foreignKey string, ids []any) (map[any]map[string]any, error) {
	if len(ids) == 0 {
		return make(map[any]map[string]any), nil
	}

	// Build IN query for related items
	builder := query.NewBuilder(relatedCollection.TableName).
		Where([]query.Filter{
			{Field: relatedCollection.PrimaryKey, Operator: query.OpIn, Value: interfacesToString(ids)},
		})

	selectSQL, selectArgs := builder.BuildSelect()
	rows, err := r.db.QueryxContext(ctx, selectSQL, selectArgs...)
	if err != nil {
		return nil, apperror.ErrInternalServer.WithError(err)
	}
	defer rows.Close()

	result := make(map[any]map[string]any)
	for rows.Next() {
		item := make(map[string]any)
		if err := rows.MapScan(item); err != nil {
			return nil, apperror.ErrInternalServer.WithError(err)
		}
		normalizeMapValues(item)
		if id, ok := item[relatedCollection.PrimaryKey]; ok {
			result[normalizeValue(id)] = item
		}
	}

	return result, nil
}

// ListOptions holds options for list queries.
type ListOptions struct {
	Filters    []query.Filter
	Sorts      []query.Sort
	Pagination query.Pagination
}

// normalizeMapValues converts []byte to string and handles other type normalizations.
func normalizeMapValues(m map[string]any) {
	for k, v := range m {
		m[k] = normalizeValue(v)
	}
}

// normalizeValue normalizes a single value.
func normalizeValue(v any) any {
	switch val := v.(type) {
	case []byte:
		return string(val)
	default:
		return val
	}
}

// interfacesToString converts a slice of interfaces to comma-separated string.
func interfacesToString(ids []any) string {
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = interfaceToString(id)
	}
	return joinStrings(strs, ",")
}

// interfaceToString converts an interface to string.
func interfaceToString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	default:
		return ""
	}
}

// joinStrings joins strings with separator.
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// isDuplicateKeyError checks if an error is a duplicate key violation.
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	// PostgreSQL error code for unique_violation is 23505
	errStr := err.Error()
	return contains(errStr, "23505") || contains(errStr, "duplicate key")
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr) >= 0))
}

// findSubstring finds the index of substr in s.
func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
