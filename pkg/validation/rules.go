package validation

import (
	"context"
	"fmt"
	"net/mail"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Required validates that a value is not nil or empty.
type Required struct{}

func (r *Required) Name() string { return "required" }

func (r *Required) Validate(ctx context.Context, value interface{}) error {
	if value == nil {
		return fmt.Errorf("field is required")
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.String:
		if strings.TrimSpace(v.String()) == "" {
			return fmt.Errorf("field is required")
		}
	case reflect.Slice, reflect.Map, reflect.Array:
		if v.Len() == 0 {
			return fmt.Errorf("field is required")
		}
	case reflect.Ptr, reflect.Interface:
		if v.IsNil() {
			return fmt.Errorf("field is required")
		}
	}

	return nil
}

// Email validates that a value is a valid email address.
type Email struct{}

func (e *Email) Name() string { return "email" }

func (e *Email) Validate(ctx context.Context, value interface{}) error {
	if value == nil {
		return nil // Use Required for nil checks
	}

	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}

	if str == "" {
		return nil // Use Required for empty checks
	}

	_, err := mail.ParseAddress(str)
	if err != nil {
		return fmt.Errorf("invalid email address")
	}

	return nil
}

// MinLength validates minimum string length.
type MinLength struct {
	Min int
}

func (m *MinLength) Name() string { return "min_length" }

func (m *MinLength) Validate(ctx context.Context, value interface{}) error {
	if value == nil {
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}

	if utf8.RuneCountInString(str) < m.Min {
		return fmt.Errorf("must be at least %d characters", m.Min)
	}

	return nil
}

// MaxLength validates maximum string length.
type MaxLength struct {
	Max int
}

func (m *MaxLength) Name() string { return "max_length" }

func (m *MaxLength) Validate(ctx context.Context, value interface{}) error {
	if value == nil {
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}

	if utf8.RuneCountInString(str) > m.Max {
		return fmt.Errorf("must be at most %d characters", m.Max)
	}

	return nil
}

// Min validates minimum numeric value.
type Min struct {
	Min float64
}

func (m *Min) Name() string { return "min" }

func (m *Min) Validate(ctx context.Context, value interface{}) error {
	if value == nil {
		return nil
	}

	num, err := toFloat64(value)
	if err != nil {
		return fmt.Errorf("must be a number")
	}

	if num < m.Min {
		return fmt.Errorf("must be at least %v", m.Min)
	}

	return nil
}

// Max validates maximum numeric value.
type Max struct {
	Max float64
}

func (m *Max) Name() string { return "max" }

func (m *Max) Validate(ctx context.Context, value interface{}) error {
	if value == nil {
		return nil
	}

	num, err := toFloat64(value)
	if err != nil {
		return fmt.Errorf("must be a number")
	}

	if num > m.Max {
		return fmt.Errorf("must be at most %v", m.Max)
	}

	return nil
}

// Range validates that a numeric value is within a range.
type Range struct {
	Min float64
	Max float64
}

func (r *Range) Name() string { return "range" }

func (r *Range) Validate(ctx context.Context, value interface{}) error {
	if value == nil {
		return nil
	}

	num, err := toFloat64(value)
	if err != nil {
		return fmt.Errorf("must be a number")
	}

	if num < r.Min || num > r.Max {
		return fmt.Errorf("must be between %v and %v", r.Min, r.Max)
	}

	return nil
}

// In validates that a value is in a list of allowed values.
type In struct {
	Values []interface{}
}

func (i *In) Name() string { return "in" }

func (i *In) Validate(ctx context.Context, value interface{}) error {
	if value == nil {
		return nil
	}

	for _, v := range i.Values {
		if reflect.DeepEqual(value, v) {
			return nil
		}
		// Also check string comparison for flexibility
		if fmt.Sprintf("%v", value) == fmt.Sprintf("%v", v) {
			return nil
		}
	}

	allowed := make([]string, len(i.Values))
	for j, v := range i.Values {
		allowed[j] = fmt.Sprintf("%v", v)
	}
	return fmt.Errorf("must be one of: %s", strings.Join(allowed, ", "))
}

// Pattern validates that a string matches a regex pattern.
type Pattern struct {
	Regex   *regexp.Regexp
	Message string
}

func (p *Pattern) Name() string { return "pattern" }

func (p *Pattern) Validate(ctx context.Context, value interface{}) error {
	if value == nil {
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}

	if str == "" {
		return nil
	}

	if !p.Regex.MatchString(str) {
		if p.Message != "" {
			return fmt.Errorf("%s", p.Message)
		}
		return fmt.Errorf("invalid format")
	}

	return nil
}

// URL validates that a string is a valid URL.
type URL struct{}

func (u *URL) Name() string { return "url" }

var urlRegex = regexp.MustCompile(`^https?://[^\s/$.?#].[^\s]*$`)

func (u *URL) Validate(ctx context.Context, value interface{}) error {
	if value == nil {
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}

	if str == "" {
		return nil
	}

	if !urlRegex.MatchString(str) {
		return fmt.Errorf("invalid URL")
	}

	return nil
}

// UUID validates that a string is a valid UUID.
type UUID struct{}

func (u *UUID) Name() string { return "uuid" }

var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (u *UUID) Validate(ctx context.Context, value interface{}) error {
	if value == nil {
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}

	if str == "" {
		return nil
	}

	if !uuidRegex.MatchString(str) {
		return fmt.Errorf("invalid UUID")
	}

	return nil
}

// Alpha validates that a string contains only letters.
type Alpha struct{}

func (a *Alpha) Name() string { return "alpha" }

var alphaRegex = regexp.MustCompile(`^[a-zA-Z]+$`)

func (a *Alpha) Validate(ctx context.Context, value interface{}) error {
	if value == nil {
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}

	if str == "" {
		return nil
	}

	if !alphaRegex.MatchString(str) {
		return fmt.Errorf("must contain only letters")
	}

	return nil
}

// AlphaNumeric validates that a string contains only letters and numbers.
type AlphaNumeric struct{}

func (a *AlphaNumeric) Name() string { return "alpha_numeric" }

var alphaNumericRegex = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

func (a *AlphaNumeric) Validate(ctx context.Context, value interface{}) error {
	if value == nil {
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}

	if str == "" {
		return nil
	}

	if !alphaNumericRegex.MatchString(str) {
		return fmt.Errorf("must contain only letters and numbers")
	}

	return nil
}

// Numeric validates that a string contains only numbers.
type Numeric struct{}

func (n *Numeric) Name() string { return "numeric" }

var numericRegex = regexp.MustCompile(`^[0-9]+$`)

func (n *Numeric) Validate(ctx context.Context, value interface{}) error {
	if value == nil {
		return nil
	}

	str, ok := value.(string)
	if !ok {
		// If it's a number type, it's valid
		if _, err := toFloat64(value); err == nil {
			return nil
		}
		return fmt.Errorf("must be numeric")
	}

	if str == "" {
		return nil
	}

	if !numericRegex.MatchString(str) {
		return fmt.Errorf("must contain only numbers")
	}

	return nil
}

// toFloat64 converts various numeric types to float64.
func toFloat64(value interface{}) (float64, error) {
	switch v := value.(type) {
	case int:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint8:
		return float64(v), nil
	case uint16:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	default:
		return 0, fmt.Errorf("not a number")
	}
}

// Helper functions to create validators

// NewRequired creates a new Required validator.
func NewRequired() *Required {
	return &Required{}
}

// NewEmail creates a new Email validator.
func NewEmail() *Email {
	return &Email{}
}

// NewMinLength creates a new MinLength validator.
func NewMinLength(min int) *MinLength {
	return &MinLength{Min: min}
}

// NewMaxLength creates a new MaxLength validator.
func NewMaxLength(max int) *MaxLength {
	return &MaxLength{Max: max}
}

// NewMin creates a new Min validator.
func NewMin(min float64) *Min {
	return &Min{Min: min}
}

// NewMax creates a new Max validator.
func NewMax(max float64) *Max {
	return &Max{Max: max}
}

// NewRange creates a new Range validator.
func NewRange(min, max float64) *Range {
	return &Range{Min: min, Max: max}
}

// NewIn creates a new In validator.
func NewIn(values ...interface{}) *In {
	return &In{Values: values}
}

// NewPattern creates a new Pattern validator.
func NewPattern(pattern string, message string) (*Pattern, error) {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}
	return &Pattern{Regex: regex, Message: message}, nil
}

// NewURL creates a new URL validator.
func NewURL() *URL {
	return &URL{}
}

// NewUUID creates a new UUID validator.
func NewUUID() *UUID {
	return &UUID{}
}

// NewAlpha creates a new Alpha validator.
func NewAlpha() *Alpha {
	return &Alpha{}
}

// NewAlphaNumeric creates a new AlphaNumeric validator.
func NewAlphaNumeric() *AlphaNumeric {
	return &AlphaNumeric{}
}

// NewNumeric creates a new Numeric validator.
func NewNumeric() *Numeric {
	return &Numeric{}
}
