package permission

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Action represents a CRUD action.
type Action string

const (
	ActionCreate Action = "create"
	ActionRead   Action = "read"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
)

// Policy represents a permission policy for a specific role and collection.
type Policy struct {
	ID               string          `db:"id" json:"id"`
	RoleID           string          `db:"role_id" json:"role_id"`
	Collection       string          `db:"collection" json:"collection"`
	Action           Action          `db:"action" json:"action"`
	Filter           json.RawMessage `db:"filter" json:"filter,omitempty"`
	FieldPermissions json.RawMessage `db:"field_permissions" json:"field_permissions,omitempty"`
	Validation       json.RawMessage `db:"validation" json:"validation,omitempty"`
	Presets          json.RawMessage `db:"presets" json:"presets,omitempty"`
	CreatedAt        time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time       `db:"updated_at" json:"updated_at"`
}

// ParsedPolicy contains the parsed permission policy data.
type ParsedPolicy struct {
	Policy
	FilterMap           map[string]any   `json:"-"`
	FieldPermissionsMap FieldPermissions `json:"-"`
	ValidationMap       map[string]any   `json:"-"`
	PresetsMap          map[string]any   `json:"-"`
}

// FieldPermissions defines field-level access control.
type FieldPermissions struct {
	// Allowed lists fields that are allowed (if not empty, acts as whitelist)
	Allowed []string `json:"allowed,omitempty"`
	// Denied lists fields that are denied (acts as blacklist)
	Denied []string `json:"denied,omitempty"`
	// ReadOnly lists fields that can only be read, not written
	ReadOnly []string `json:"read_only,omitempty"`
}

// FilterRule represents a filter condition for row-level security.
type FilterRule struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"` // eq, ne, gt, gte, lt, lte, in, like, null, notnull
	Value    any         `json:"value"`
	Variable string      `json:"variable,omitempty"` // $USER_ID, $ROLE_ID, $NOW, etc.
}

// PolicyStore provides storage operations for policies.
type PolicyStore struct {
	db        *sqlx.DB
	tableName string
}

// NewPolicyStore creates a new policy store.
func NewPolicyStore(db *sqlx.DB) *PolicyStore {
	return &PolicyStore{
		db:        db,
		tableName: "tugo_permissions",
	}
}

// GetByRoleAndCollection retrieves a policy by role ID, collection, and action.
func (s *PolicyStore) GetByRoleAndCollection(ctx context.Context, roleID, collection string, action Action) (*Policy, error) {
	query := `
		SELECT id, role_id, collection, action, filter, field_permissions, validation, presets, created_at, updated_at
		FROM ` + s.tableName + `
		WHERE role_id = $1 AND collection = $2 AND action = $3
	`

	var policy Policy
	if err := s.db.GetContext(ctx, &policy, query, roleID, collection, action); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // No policy found
		}
		return nil, err
	}

	return &policy, nil
}

// GetByRole retrieves all policies for a role.
func (s *PolicyStore) GetByRole(ctx context.Context, roleID string) ([]Policy, error) {
	query := `
		SELECT id, role_id, collection, action, filter, field_permissions, validation, presets, created_at, updated_at
		FROM ` + s.tableName + `
		WHERE role_id = $1
		ORDER BY collection, action
	`

	var policies []Policy
	if err := s.db.SelectContext(ctx, &policies, query, roleID); err != nil {
		return nil, err
	}

	return policies, nil
}

// GetByCollection retrieves all policies for a collection.
func (s *PolicyStore) GetByCollection(ctx context.Context, collection string) ([]Policy, error) {
	query := `
		SELECT id, role_id, collection, action, filter, field_permissions, validation, presets, created_at, updated_at
		FROM ` + s.tableName + `
		WHERE collection = $1
		ORDER BY role_id, action
	`

	var policies []Policy
	if err := s.db.SelectContext(ctx, &policies, query, collection); err != nil {
		return nil, err
	}

	return policies, nil
}

// Create creates a new policy.
func (s *PolicyStore) Create(ctx context.Context, policy *Policy) error {
	if policy.ID == "" {
		policy.ID = uuid.New().String()
	}
	now := time.Now()
	policy.CreatedAt = now
	policy.UpdatedAt = now

	query := `
		INSERT INTO ` + s.tableName + ` (id, role_id, collection, action, filter, field_permissions, validation, presets, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err := s.db.ExecContext(ctx, query,
		policy.ID, policy.RoleID, policy.Collection, policy.Action,
		policy.Filter, policy.FieldPermissions, policy.Validation, policy.Presets,
		policy.CreatedAt, policy.UpdatedAt)
	return err
}

// Update updates an existing policy.
func (s *PolicyStore) Update(ctx context.Context, policy *Policy) error {
	policy.UpdatedAt = time.Now()

	query := `
		UPDATE ` + s.tableName + `
		SET filter = $1, field_permissions = $2, validation = $3, presets = $4, updated_at = $5
		WHERE id = $6
	`

	result, err := s.db.ExecContext(ctx, query,
		policy.Filter, policy.FieldPermissions, policy.Validation, policy.Presets,
		policy.UpdatedAt, policy.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.New("policy not found")
	}

	return nil
}

// Delete deletes a policy.
func (s *PolicyStore) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM ` + s.tableName + ` WHERE id = $1`

	result, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.New("policy not found")
	}

	return nil
}

// Upsert creates or updates a policy.
func (s *PolicyStore) Upsert(ctx context.Context, policy *Policy) error {
	if policy.ID == "" {
		policy.ID = uuid.New().String()
	}
	now := time.Now()
	policy.UpdatedAt = now

	query := `
		INSERT INTO ` + s.tableName + ` (id, role_id, collection, action, filter, field_permissions, validation, presets, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (role_id, collection, action)
		DO UPDATE SET filter = EXCLUDED.filter, field_permissions = EXCLUDED.field_permissions,
		              validation = EXCLUDED.validation, presets = EXCLUDED.presets, updated_at = EXCLUDED.updated_at
	`

	_, err := s.db.ExecContext(ctx, query,
		policy.ID, policy.RoleID, policy.Collection, policy.Action,
		policy.Filter, policy.FieldPermissions, policy.Validation, policy.Presets,
		now, now)
	return err
}

// ParsePolicy parses JSON fields in a policy.
func ParsePolicy(p *Policy) (*ParsedPolicy, error) {
	parsed := &ParsedPolicy{
		Policy: *p,
	}

	if len(p.Filter) > 0 {
		if err := json.Unmarshal(p.Filter, &parsed.FilterMap); err != nil {
			return nil, err
		}
	}

	if len(p.FieldPermissions) > 0 {
		if err := json.Unmarshal(p.FieldPermissions, &parsed.FieldPermissionsMap); err != nil {
			return nil, err
		}
	}

	if len(p.Validation) > 0 {
		if err := json.Unmarshal(p.Validation, &parsed.ValidationMap); err != nil {
			return nil, err
		}
	}

	if len(p.Presets) > 0 {
		if err := json.Unmarshal(p.Presets, &parsed.PresetsMap); err != nil {
			return nil, err
		}
	}

	return parsed, nil
}
