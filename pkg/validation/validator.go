package validation

import (
	"context"
	"fmt"
	"strings"
)

// Validator is the interface for field validators.
type Validator interface {
	// Validate validates a value and returns an error if invalid.
	Validate(ctx context.Context, value interface{}) error

	// Name returns the validator name for error messages.
	Name() string
}

// FieldError represents a validation error for a specific field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

func (e *FieldError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors is a collection of field errors.
type ValidationErrors struct {
	Errors []FieldError `json:"errors"`
}

func (e *ValidationErrors) Error() string {
	if len(e.Errors) == 0 {
		return "validation failed"
	}
	msgs := make([]string, len(e.Errors))
	for i, err := range e.Errors {
		msgs[i] = err.Error()
	}
	return strings.Join(msgs, "; ")
}

// Add adds a field error.
func (e *ValidationErrors) Add(field, message, code string) {
	e.Errors = append(e.Errors, FieldError{
		Field:   field,
		Message: message,
		Code:    code,
	})
}

// HasErrors returns true if there are validation errors.
func (e *ValidationErrors) HasErrors() bool {
	return len(e.Errors) > 0
}

// FieldValidator holds validators for a single field.
type FieldValidator struct {
	field      string
	validators []Validator
}

// NewFieldValidator creates a new field validator.
func NewFieldValidator(field string) *FieldValidator {
	return &FieldValidator{
		field:      field,
		validators: make([]Validator, 0),
	}
}

// Add adds a validator to the chain.
func (fv *FieldValidator) Add(v Validator) *FieldValidator {
	fv.validators = append(fv.validators, v)
	return fv
}

// Validate validates a value against all validators.
func (fv *FieldValidator) Validate(ctx context.Context, value interface{}) *FieldError {
	for _, v := range fv.validators {
		if err := v.Validate(ctx, value); err != nil {
			return &FieldError{
				Field:   fv.field,
				Message: err.Error(),
				Code:    v.Name(),
			}
		}
	}
	return nil
}

// ValidatePartial validates a value but skips "required" validation.
// This is used for partial updates (PATCH) where not all fields are provided.
func (fv *FieldValidator) ValidatePartial(ctx context.Context, value interface{}) *FieldError {
	for _, v := range fv.validators {
		// Skip required validation for partial updates
		if v.Name() == "required" {
			continue
		}
		if err := v.Validate(ctx, value); err != nil {
			return &FieldError{
				Field:   fv.field,
				Message: err.Error(),
				Code:    v.Name(),
			}
		}
	}
	return nil
}

// Schema holds validation rules for a collection.
type Schema struct {
	fields map[string]*FieldValidator
}

// NewSchema creates a new validation schema.
func NewSchema() *Schema {
	return &Schema{
		fields: make(map[string]*FieldValidator),
	}
}

// Field gets or creates a field validator.
func (s *Schema) Field(name string) *FieldValidator {
	if fv, ok := s.fields[name]; ok {
		return fv
	}
	fv := NewFieldValidator(name)
	s.fields[name] = fv
	return fv
}

// Validate validates data against the schema.
func (s *Schema) Validate(ctx context.Context, data map[string]interface{}) *ValidationErrors {
	errors := &ValidationErrors{}

	for fieldName, fv := range s.fields {
		value := data[fieldName]
		if err := fv.Validate(ctx, value); err != nil {
			errors.Errors = append(errors.Errors, *err)
		}
	}

	if errors.HasErrors() {
		return errors
	}
	return nil
}

// ValidatePartial validates only fields present in data (for partial updates).
// This skips "required" validation for fields not present in the input.
func (s *Schema) ValidatePartial(ctx context.Context, data map[string]interface{}) *ValidationErrors {
	errors := &ValidationErrors{}

	for fieldName, fv := range s.fields {
		// Only validate fields that are explicitly provided in data
		if value, exists := data[fieldName]; exists {
			if err := fv.ValidatePartial(ctx, value); err != nil {
				errors.Errors = append(errors.Errors, *err)
			}
		}
	}

	if errors.HasErrors() {
		return errors
	}
	return nil
}

// ValidatorFunc is an adapter to allow regular functions as Validators.
type ValidatorFunc struct {
	name string
	fn   func(ctx context.Context, value interface{}) error
}

// Validate implements Validator.
func (vf *ValidatorFunc) Validate(ctx context.Context, value interface{}) error {
	return vf.fn(ctx, value)
}

// Name implements Validator.
func (vf *ValidatorFunc) Name() string {
	return vf.name
}

// NewValidatorFunc creates a new ValidatorFunc.
func NewValidatorFunc(name string, fn func(ctx context.Context, value interface{}) error) *ValidatorFunc {
	return &ValidatorFunc{name: name, fn: fn}
}
