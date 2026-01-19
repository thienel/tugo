package validation

import (
	"context"
	"fmt"
	"regexp"

	"github.com/jmoiron/sqlx"
	"github.com/thienel/tugo/pkg/schema"
)

// CollectionValidator validates data for a specific collection.
type CollectionValidator struct {
	collection    *schema.Collection
	schema        *Schema
	uniqueChecker UniqueChecker
	db            *sqlx.DB
}

// NewCollectionValidator creates a new collection validator.
func NewCollectionValidator(collection *schema.Collection, db *sqlx.DB) *CollectionValidator {
	return &CollectionValidator{
		collection:    collection,
		schema:        NewSchema(),
		uniqueChecker: NewDBUniqueChecker(db, collection.PrimaryKey),
		db:            db,
	}
}

// Field gets a field validator for chaining.
func (cv *CollectionValidator) Field(name string) *FieldValidator {
	return cv.schema.Field(name)
}

// Validate validates data against the collection schema.
func (cv *CollectionValidator) Validate(ctx context.Context, data map[string]interface{}) *ValidationErrors {
	return cv.schema.Validate(ctx, data)
}

// ValidateForCreate validates data for creating a new record.
func (cv *CollectionValidator) ValidateForCreate(ctx context.Context, data map[string]interface{}) *ValidationErrors {
	return cv.Validate(ctx, data)
}

// ValidateForUpdate validates data for updating an existing record.
func (cv *CollectionValidator) ValidateForUpdate(ctx context.Context, id interface{}, data map[string]interface{}) *ValidationErrors {
	// For updates, we need to set the exclude ID on unique validators
	// This is handled at the validator level
	return cv.Validate(ctx, data)
}

// BuildFromSchema builds validators from the collection's field definitions.
func (cv *CollectionValidator) BuildFromSchema() *CollectionValidator {
	for _, field := range cv.collection.Fields {
		fv := cv.Field(field.Name)

		// Required validation (from nullable)
		if !field.IsNullable && !field.IsPrimaryKey {
			fv.Add(NewRequired())
		}

		// Length validation for strings
		if field.MaxLength != nil && *field.MaxLength > 0 {
			fv.Add(NewMaxLength(*field.MaxLength))
		}

		// Unique validation
		if field.IsUnique && !field.IsPrimaryKey {
			fv.Add(NewUnique(cv.uniqueChecker, cv.collection.TableName, field.Name))
		}

		// Type-based validation
		switch field.DataType {
		case "uuid":
			fv.Add(NewUUID())
		}

		// Validation rules from field metadata
		if field.ValidationRules != nil {
			cv.applyValidationRules(fv, field.ValidationRules)
		}
	}

	return cv
}

// applyValidationRules applies validation rules from field metadata.
func (cv *CollectionValidator) applyValidationRules(fv *FieldValidator, rules map[string]interface{}) {
	for ruleName, ruleValue := range rules {
		switch ruleName {
		case "required":
			if v, ok := ruleValue.(bool); ok && v {
				fv.Add(NewRequired())
			}
		case "email":
			if v, ok := ruleValue.(bool); ok && v {
				fv.Add(NewEmail())
			}
		case "url":
			if v, ok := ruleValue.(bool); ok && v {
				fv.Add(NewURL())
			}
		case "min":
			if v, ok := toFloat64Value(ruleValue); ok {
				fv.Add(NewMin(v))
			}
		case "max":
			if v, ok := toFloat64Value(ruleValue); ok {
				fv.Add(NewMax(v))
			}
		case "min_length":
			if v, ok := toIntValue(ruleValue); ok {
				fv.Add(NewMinLength(v))
			}
		case "max_length":
			if v, ok := toIntValue(ruleValue); ok {
				fv.Add(NewMaxLength(v))
			}
		case "pattern":
			if v, ok := ruleValue.(string); ok {
				if p, err := NewPattern(v, ""); err == nil {
					fv.Add(p)
				}
			}
		case "in":
			if values, ok := ruleValue.([]interface{}); ok {
				fv.Add(NewIn(values...))
			}
		case "alpha":
			if v, ok := ruleValue.(bool); ok && v {
				fv.Add(NewAlpha())
			}
		case "alpha_numeric":
			if v, ok := ruleValue.(bool); ok && v {
				fv.Add(NewAlphaNumeric())
			}
		case "numeric":
			if v, ok := ruleValue.(bool); ok && v {
				fv.Add(NewNumeric())
			}
		}
	}
}

func toFloat64Value(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	default:
		return 0, false
	}
}

func toIntValue(v interface{}) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	default:
		return 0, false
	}
}

// ValidatorRegistry holds validators for all collections.
type ValidatorRegistry struct {
	validators map[string]*CollectionValidator
	db         *sqlx.DB
}

// NewValidatorRegistry creates a new validator registry.
func NewValidatorRegistry(db *sqlx.DB) *ValidatorRegistry {
	return &ValidatorRegistry{
		validators: make(map[string]*CollectionValidator),
		db:         db,
	}
}

// Register registers a collection validator.
func (r *ValidatorRegistry) Register(collectionName string, cv *CollectionValidator) {
	r.validators[collectionName] = cv
}

// Get returns a collection validator.
func (r *ValidatorRegistry) Get(collectionName string) (*CollectionValidator, bool) {
	cv, ok := r.validators[collectionName]
	return cv, ok
}

// BuildFromCollection builds and registers a validator for a collection.
func (r *ValidatorRegistry) BuildFromCollection(collection *schema.Collection) *CollectionValidator {
	cv := NewCollectionValidator(collection, r.db)
	cv.BuildFromSchema()
	r.Register(collection.Name, cv)
	return cv
}

// Validate validates data for a collection.
func (r *ValidatorRegistry) Validate(ctx context.Context, collectionName string, data map[string]interface{}) *ValidationErrors {
	cv, ok := r.Get(collectionName)
	if !ok {
		return nil // No validation configured
	}
	return cv.Validate(ctx, data)
}

// ValidFieldName validates that a field name is safe.
var ValidFieldName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// ValidateFieldName validates a field name.
func ValidateFieldName(name string) error {
	if !ValidFieldName.MatchString(name) {
		return fmt.Errorf("invalid field name: %s", name)
	}
	return nil
}

// ValidCollectionName validates a collection name.
var ValidCollectionName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// ValidateCollectionName validates a collection name.
func ValidateCollectionName(name string) error {
	if !ValidCollectionName.MatchString(name) {
		return fmt.Errorf("invalid collection name: %s", name)
	}
	return nil
}
