package permission

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/jmoiron/sqlx"
	"github.com/thienel/tugo/pkg/auth"
	"go.uber.org/zap"
)

// Checker handles permission checking for collections.
type Checker struct {
	db     *sqlx.DB
	store  *PolicyStore
	logger *zap.SugaredLogger
	cache  *policyCache
}

// policyCache caches policies by role ID.
type policyCache struct {
	mu       sync.RWMutex
	policies map[string][]Policy // roleID -> policies
}

// NewChecker creates a new permission checker.
func NewChecker(db *sqlx.DB, logger *zap.SugaredLogger) *Checker {
	return &Checker{
		db:     db,
		store:  NewPolicyStore(db),
		logger: logger,
		cache: &policyCache{
			policies: make(map[string][]Policy),
		},
	}
}

// CheckResult contains the result of a permission check.
type CheckResult struct {
	Allowed    bool
	Filter     map[string]any   // Row-level filter to apply
	FieldPerms FieldPermissions // Field-level permissions
	Presets    map[string]any   // Default values to apply on create
	Reason     string           // Reason if not allowed
}

// Check checks if a user has permission to perform an action on a collection.
func (c *Checker) Check(ctx context.Context, user *auth.User, collection string, action Action) (*CheckResult, error) {
	if user == nil {
		return &CheckResult{
			Allowed: false,
			Reason:  "user not authenticated",
		}, nil
	}

	// Admin role has full access
	if user.Role == "admin" {
		return &CheckResult{
			Allowed: true,
		}, nil
	}

	// Get policy for user's role
	policy, err := c.getPolicy(ctx, user.RoleID, collection, action)
	if err != nil {
		return nil, fmt.Errorf("failed to get policy: %w", err)
	}

	// No policy means no permission
	if policy == nil {
		// Check wildcard policy
		policy, err = c.getPolicy(ctx, user.RoleID, "*", action)
		if err != nil {
			return nil, fmt.Errorf("failed to get wildcard policy: %w", err)
		}

		if policy == nil {
			return &CheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("no permission for %s on %s", action, collection),
			}, nil
		}
	}

	// Parse policy
	parsed, err := ParsePolicy(policy)
	if err != nil {
		return nil, fmt.Errorf("failed to parse policy: %w", err)
	}

	// Resolve filter variables
	resolvedFilter := c.resolveFilterVariables(parsed.FilterMap, user)

	return &CheckResult{
		Allowed:    true,
		Filter:     resolvedFilter,
		FieldPerms: parsed.FieldPermissionsMap,
		Presets:    parsed.PresetsMap,
	}, nil
}

// CheckWithData checks permission and validates data against policy.
func (c *Checker) CheckWithData(ctx context.Context, user *auth.User, collection string, action Action, data map[string]any) (*CheckResult, error) {
	result, err := c.Check(ctx, user, collection, action)
	if err != nil {
		return nil, err
	}

	if !result.Allowed {
		return result, nil
	}

	// Check field-level permissions
	if err := c.checkFieldPermissions(data, result.FieldPerms, action); err != nil {
		return &CheckResult{
			Allowed: false,
			Reason:  err.Error(),
		}, nil
	}

	// Apply presets for create action
	if action == ActionCreate && len(result.Presets) > 0 {
		for key, value := range result.Presets {
			// Only apply preset if field is not provided
			if _, exists := data[key]; !exists {
				data[key] = c.resolvePresetValue(value, user)
			}
		}
	}

	return result, nil
}

// FilterAllowedFields filters data to only include allowed fields.
func (c *Checker) FilterAllowedFields(data map[string]any, perms FieldPermissions, action Action) map[string]any {
	if len(perms.Allowed) == 0 && len(perms.Denied) == 0 {
		return data // No restrictions
	}

	result := make(map[string]any)

	for key, value := range data {
		// Check if field is explicitly denied
		if contains(perms.Denied, key) {
			continue
		}

		// Check if field is read-only and we're writing
		if action != ActionRead && contains(perms.ReadOnly, key) {
			continue
		}

		// If whitelist is specified, check if field is allowed
		if len(perms.Allowed) > 0 && !contains(perms.Allowed, key) {
			continue
		}

		result[key] = value
	}

	return result
}

// getPolicy retrieves a policy from cache or database.
func (c *Checker) getPolicy(ctx context.Context, roleID, collection string, action Action) (*Policy, error) {
	// Try to get from cache first
	c.cache.mu.RLock()
	policies, ok := c.cache.policies[roleID]
	c.cache.mu.RUnlock()

	if ok {
		for i := range policies {
			if policies[i].Collection == collection && policies[i].Action == action {
				return &policies[i], nil
			}
		}
		return nil, nil
	}

	// Fetch from database
	policy, err := c.store.GetByRoleAndCollection(ctx, roleID, collection, action)
	if err != nil {
		return nil, err
	}

	return policy, nil
}

// LoadRolePolicies loads all policies for a role into cache.
func (c *Checker) LoadRolePolicies(ctx context.Context, roleID string) error {
	policies, err := c.store.GetByRole(ctx, roleID)
	if err != nil {
		return err
	}

	c.cache.mu.Lock()
	c.cache.policies[roleID] = policies
	c.cache.mu.Unlock()

	return nil
}

// ClearCache clears the policy cache.
func (c *Checker) ClearCache() {
	c.cache.mu.Lock()
	c.cache.policies = make(map[string][]Policy)
	c.cache.mu.Unlock()
}

// checkFieldPermissions validates that data doesn't contain disallowed fields.
func (c *Checker) checkFieldPermissions(data map[string]any, perms FieldPermissions, action Action) error {
	for key := range data {
		// Check denied fields
		if contains(perms.Denied, key) {
			return fmt.Errorf("field '%s' is not allowed", key)
		}

		// Check read-only fields for write operations
		if action != ActionRead && contains(perms.ReadOnly, key) {
			return fmt.Errorf("field '%s' is read-only", key)
		}

		// Check whitelist
		if len(perms.Allowed) > 0 && !contains(perms.Allowed, key) {
			return fmt.Errorf("field '%s' is not in allowed list", key)
		}
	}

	return nil
}

// resolveFilterVariables replaces variables in filter with actual values.
func (c *Checker) resolveFilterVariables(filter map[string]any, user *auth.User) map[string]any {
	if filter == nil {
		return nil
	}

	result := make(map[string]any)
	for key, value := range filter {
		result[key] = c.resolveValue(value, user)
	}
	return result
}

// resolveValue resolves a single value, handling nested maps and variables.
func (c *Checker) resolveValue(value any, user *auth.User) any {
	switch v := value.(type) {
	case string:
		return c.resolveVariable(v, user)
	case map[string]any:
		result := make(map[string]any)
		for k, val := range v {
			result[k] = c.resolveValue(val, user)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, val := range v {
			result[i] = c.resolveValue(val, user)
		}
		return result
	default:
		return value
	}
}

// resolveVariable resolves special variables.
func (c *Checker) resolveVariable(value string, user *auth.User) any {
	switch value {
	case "$USER_ID", "$CURRENT_USER":
		return user.ID
	case "$ROLE_ID", "$CURRENT_ROLE":
		return user.RoleID
	case "$ROLE", "$ROLE_NAME":
		return user.Role
	case "$USERNAME":
		return user.Username
	case "$EMAIL":
		return user.Email
	default:
		return value
	}
}

// resolvePresetValue resolves preset values.
func (c *Checker) resolvePresetValue(value any, user *auth.User) any {
	if strVal, ok := value.(string); ok {
		return c.resolveVariable(strVal, user)
	}
	return value
}

// contains checks if a string slice contains a value.
func contains(slice []string, value string) bool {
	for _, s := range slice {
		if s == value {
			return true
		}
	}
	return false
}

// CreatePolicy creates a new policy.
func (c *Checker) CreatePolicy(ctx context.Context, policy *Policy) error {
	return c.store.Create(ctx, policy)
}

// UpdatePolicy updates an existing policy.
func (c *Checker) UpdatePolicy(ctx context.Context, policy *Policy) error {
	return c.store.Update(ctx, policy)
}

// DeletePolicy deletes a policy.
func (c *Checker) DeletePolicy(ctx context.Context, id string) error {
	return c.store.Delete(ctx, id)
}

// GetPoliciesForRole returns all policies for a role.
func (c *Checker) GetPoliciesForRole(ctx context.Context, roleID string) ([]Policy, error) {
	return c.store.GetByRole(ctx, roleID)
}

// GetPoliciesForCollection returns all policies for a collection.
func (c *Checker) GetPoliciesForCollection(ctx context.Context, collection string) ([]Policy, error) {
	return c.store.GetByCollection(ctx, collection)
}

// SetPolicy sets or updates a policy for a role/collection/action combination.
func (c *Checker) SetPolicy(ctx context.Context, roleID, collection string, action Action, filter, fieldPerms, presets map[string]any) error {
	var filterJSON, fieldPermsJSON, presetsJSON []byte
	var err error

	if filter != nil {
		filterJSON, err = json.Marshal(filter)
		if err != nil {
			return fmt.Errorf("failed to marshal filter: %w", err)
		}
	}

	if fieldPerms != nil {
		fieldPermsJSON, err = json.Marshal(fieldPerms)
		if err != nil {
			return fmt.Errorf("failed to marshal field permissions: %w", err)
		}
	}

	if presets != nil {
		presetsJSON, err = json.Marshal(presets)
		if err != nil {
			return fmt.Errorf("failed to marshal presets: %w", err)
		}
	}

	policy := &Policy{
		RoleID:           roleID,
		Collection:       collection,
		Action:           action,
		Filter:           filterJSON,
		FieldPermissions: fieldPermsJSON,
		Presets:          presetsJSON,
	}

	return c.store.Upsert(ctx, policy)
}
