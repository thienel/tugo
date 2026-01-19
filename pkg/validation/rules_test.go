package validation

import (
	"context"
	"testing"
)

func TestRequired_Validate(t *testing.T) {
	v := NewRequired()
	ctx := context.Background()

	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{"nil value", nil, true},
		{"empty string", "", true},
		{"whitespace only", "   ", true},
		{"valid string", "hello", false},
		{"zero int", 0, false},
		{"non-zero int", 42, false},
		{"empty slice", []string{}, true},
		{"non-empty slice", []string{"a"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(ctx, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Required.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRequired_Name(t *testing.T) {
	v := NewRequired()
	if v.Name() != "required" {
		t.Errorf("expected name 'required', got '%s'", v.Name())
	}
}

func TestEmail_Validate(t *testing.T) {
	v := NewEmail()
	ctx := context.Background()

	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{"valid email", "test@example.com", false},
		{"valid email with name", "John Doe <john@example.com>", false},
		{"invalid email no domain", "test@", true},
		{"invalid email no at", "testexample.com", true},
		{"empty string", "", false}, // Empty is allowed - use Required for that
		{"nil value", nil, false},   // Nil is allowed - use Required for that
		{"not a string", 123, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(ctx, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Email.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMinLength_Validate(t *testing.T) {
	v := NewMinLength(3)
	ctx := context.Background()

	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{"exactly min length", "abc", false},
		{"above min length", "abcd", false},
		{"below min length", "ab", true},
		{"empty string", "", true},
		{"unicode characters", "abc", false},
		{"unicode below min", "日本", true}, // 2 runes
		{"nil value", nil, false},
		{"not a string", 123, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(ctx, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("MinLength.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMaxLength_Validate(t *testing.T) {
	v := NewMaxLength(5)
	ctx := context.Background()

	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{"exactly max length", "abcde", false},
		{"below max length", "abc", false},
		{"above max length", "abcdef", true},
		{"empty string", "", false},
		{"nil value", nil, false},
		{"not a string", 123, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(ctx, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("MaxLength.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMin_Validate(t *testing.T) {
	v := NewMin(10)
	ctx := context.Background()

	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{"exactly min", 10, false},
		{"above min", 15, false},
		{"below min", 5, true},
		{"float above min", 10.5, false},
		{"float below min", 9.9, true},
		{"nil value", nil, false},
		{"not a number", "abc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(ctx, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Min.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMax_Validate(t *testing.T) {
	v := NewMax(100)
	ctx := context.Background()

	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{"exactly max", 100, false},
		{"below max", 50, false},
		{"above max", 150, true},
		{"float below max", 99.9, false},
		{"float above max", 100.1, true},
		{"nil value", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(ctx, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Max.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRange_Validate(t *testing.T) {
	v := NewRange(18, 65)
	ctx := context.Background()

	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{"at lower bound", 18, false},
		{"at upper bound", 65, false},
		{"in range", 30, false},
		{"below range", 17, true},
		{"above range", 66, true},
		{"nil value", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(ctx, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Range.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIn_Validate(t *testing.T) {
	v := NewIn("active", "pending", "completed")
	ctx := context.Background()

	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{"valid value 1", "active", false},
		{"valid value 2", "pending", false},
		{"valid value 3", "completed", false},
		{"invalid value", "deleted", true},
		{"nil value", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(ctx, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("In.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUUID_Validate(t *testing.T) {
	v := NewUUID()
	ctx := context.Background()

	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{"valid uuid v4", "550e8400-e29b-41d4-a716-446655440000", false},
		{"valid uuid lowercase", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", false},
		{"valid uuid uppercase", "A0EEBC99-9C0B-4EF8-BB6D-6BB9BD380A11", false},
		{"invalid format", "not-a-uuid", true},
		{"missing hyphens", "550e8400e29b41d4a716446655440000", true},
		{"empty string", "", false},
		{"nil value", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(ctx, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("UUID.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestURL_Validate(t *testing.T) {
	v := NewURL()
	ctx := context.Background()

	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{"valid http url", "http://example.com", false},
		{"valid https url", "https://example.com", false},
		{"valid url with path", "https://example.com/path", false},
		{"valid url with query", "https://example.com?q=test", false},
		{"missing protocol", "example.com", true},
		{"ftp protocol", "ftp://example.com", true},
		{"empty string", "", false},
		{"nil value", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(ctx, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("URL.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPattern_Validate(t *testing.T) {
	v, err := NewPattern(`^[A-Z]{2,3}$`, "must be 2-3 uppercase letters")
	if err != nil {
		t.Fatalf("failed to create pattern: %v", err)
	}
	ctx := context.Background()

	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{"valid 2 letters", "AB", false},
		{"valid 3 letters", "ABC", false},
		{"lowercase", "abc", true},
		{"too short", "A", true},
		{"too long", "ABCD", true},
		{"empty string", "", false},
		{"nil value", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(ctx, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Pattern.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewPattern_InvalidRegex(t *testing.T) {
	_, err := NewPattern(`[invalid`, "message")
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestAlpha_Validate(t *testing.T) {
	v := NewAlpha()
	ctx := context.Background()

	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{"letters only", "Hello", false},
		{"with numbers", "Hello123", true},
		{"with special chars", "Hello!", true},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(ctx, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Alpha.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAlphaNumeric_Validate(t *testing.T) {
	v := NewAlphaNumeric()
	ctx := context.Background()

	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{"letters only", "Hello", false},
		{"numbers only", "123", false},
		{"alphanumeric", "Hello123", false},
		{"with special chars", "Hello!", true},
		{"with space", "Hello World", true},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(ctx, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("AlphaNumeric.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNumeric_Validate(t *testing.T) {
	v := NewNumeric()
	ctx := context.Background()

	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{"numbers string", "123", false},
		{"with letters", "123abc", true},
		{"with decimal", "12.3", true},
		{"negative string", "-123", true},
		{"actual int", 123, false},
		{"actual float", 12.3, false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(ctx, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Numeric.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
