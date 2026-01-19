package auth

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// mockUserStore implements UserStore for testing
type mockUserStore struct {
	users         map[string]*User
	passwordHash  string
	returnError   error
}

func newMockUserStore() *mockUserStore {
	return &mockUserStore{
		users: make(map[string]*User),
	}
}

func (m *mockUserStore) GetByID(ctx context.Context, id string) (*User, error) {
	if m.returnError != nil {
		return nil, m.returnError
	}
	if user, ok := m.users[id]; ok {
		return user, nil
	}
	return nil, nil
}

func (m *mockUserStore) GetByUsername(ctx context.Context, username string) (*User, error) {
	if m.returnError != nil {
		return nil, m.returnError
	}
	for _, user := range m.users {
		if user.Username == username {
			return user, nil
		}
	}
	return nil, nil
}

func (m *mockUserStore) GetByEmail(ctx context.Context, email string) (*User, error) {
	if m.returnError != nil {
		return nil, m.returnError
	}
	for _, user := range m.users {
		if user.Email == email {
			return user, nil
		}
	}
	return nil, nil
}

func (m *mockUserStore) GetPasswordHash(ctx context.Context, userID string) (string, error) {
	if m.returnError != nil {
		return "", m.returnError
	}
	return m.passwordHash, nil
}

func (m *mockUserStore) GetTOTPSecret(ctx context.Context, userID string) (string, error) {
	return "", nil
}

func (m *mockUserStore) Create(ctx context.Context, user *User, passwordHash string) error {
	m.users[user.ID] = user
	m.passwordHash = passwordHash
	return nil
}

func (m *mockUserStore) UpdatePassword(ctx context.Context, userID string, passwordHash string) error {
	m.passwordHash = passwordHash
	return nil
}

func (m *mockUserStore) SetTOTPSecret(ctx context.Context, userID string, secret string) error {
	return nil
}

func (m *mockUserStore) EnableTOTP(ctx context.Context, userID string, enabled bool) error {
	return nil
}

func TestJWTProvider_GenerateTokens(t *testing.T) {
	store := newMockUserStore()
	config := JWTConfig{
		Secret:        "test-secret-key-min-32-characters",
		Expiry:        3600,  // 1 hour
		RefreshExpiry: 86400, // 24 hours
		Issuer:        "test-issuer",
	}
	provider := NewJWTProvider(config, store)

	user := &User{
		ID:       "user-123",
		Username: "testuser",
		Role:     "admin",
		RoleID:   "role-456",
		Status:   "active",
	}

	tokens, err := provider.GenerateTokens(context.Background(), user)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tokens.AccessToken == "" {
		t.Error("access token should not be empty")
	}

	if tokens.RefreshToken == "" {
		t.Error("refresh token should not be empty")
	}

	if tokens.TokenType != "Bearer" {
		t.Errorf("expected token type 'Bearer', got '%s'", tokens.TokenType)
	}

	if tokens.ExpiresIn != 3600 {
		t.Errorf("expected expires_in 3600, got %d", tokens.ExpiresIn)
	}
}

func TestJWTProvider_ValidateToken(t *testing.T) {
	store := newMockUserStore()
	config := JWTConfig{
		Secret:        "test-secret-key-min-32-characters",
		Expiry:        3600,
		RefreshExpiry: 86400,
		Issuer:        "test-issuer",
	}
	provider := NewJWTProvider(config, store)

	user := &User{
		ID:       "user-123",
		Username: "testuser",
		Role:     "admin",
		RoleID:   "role-456",
	}

	tokens, _ := provider.GenerateTokens(context.Background(), user)

	claims, err := provider.ValidateToken(context.Background(), tokens.AccessToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if claims.UserID != user.ID {
		t.Errorf("expected user ID '%s', got '%s'", user.ID, claims.UserID)
	}

	if claims.Username != user.Username {
		t.Errorf("expected username '%s', got '%s'", user.Username, claims.Username)
	}

	if claims.Role != user.Role {
		t.Errorf("expected role '%s', got '%s'", user.Role, claims.Role)
	}
}

func TestJWTProvider_ValidateToken_RefreshTokenRejected(t *testing.T) {
	store := newMockUserStore()
	config := JWTConfig{
		Secret:        "test-secret-key-min-32-characters",
		Expiry:        3600,
		RefreshExpiry: 86400,
		Issuer:        "test-issuer",
	}
	provider := NewJWTProvider(config, store)

	user := &User{
		ID:       "user-123",
		Username: "testuser",
		Role:     "user",
	}

	tokens, _ := provider.GenerateTokens(context.Background(), user)

	// ValidateToken should reject refresh tokens
	_, err := provider.ValidateToken(context.Background(), tokens.RefreshToken)
	if err == nil {
		t.Error("expected error when validating refresh token as access token")
	}
}

func TestJWTProvider_ValidateToken_InvalidToken(t *testing.T) {
	store := newMockUserStore()
	config := JWTConfig{
		Secret:        "test-secret-key-min-32-characters",
		Expiry:        3600,
		RefreshExpiry: 86400,
		Issuer:        "test-issuer",
	}
	provider := NewJWTProvider(config, store)

	_, err := provider.ValidateToken(context.Background(), "invalid.token.here")
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestJWTProvider_ValidateToken_WrongSecret(t *testing.T) {
	store := newMockUserStore()
	config1 := JWTConfig{
		Secret:        "secret-one-min-32-characters-long",
		Expiry:        3600,
		RefreshExpiry: 86400,
	}
	config2 := JWTConfig{
		Secret:        "secret-two-min-32-characters-long",
		Expiry:        3600,
		RefreshExpiry: 86400,
	}
	provider1 := NewJWTProvider(config1, store)
	provider2 := NewJWTProvider(config2, store)

	user := &User{ID: "user-123", Username: "test"}
	tokens, _ := provider1.GenerateTokens(context.Background(), user)

	_, err := provider2.ValidateToken(context.Background(), tokens.AccessToken)
	if err == nil {
		t.Error("expected error when validating with wrong secret")
	}
}

func TestJWTProvider_ValidateToken_Expired(t *testing.T) {
	store := newMockUserStore()
	config := JWTConfig{
		Secret:        "test-secret-key-min-32-characters",
		Expiry:        -1, // Already expired
		RefreshExpiry: 86400,
		Issuer:        "test-issuer",
	}
	provider := NewJWTProvider(config, store)

	// Create an expired token manually
	now := time.Now()
	claims := JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "test-issuer",
			Subject:   "user-123",
			IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-1 * time.Hour)),
		},
		UserID: "user-123",
		Type:   "access",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(config.Secret))

	_, err := provider.ValidateToken(context.Background(), tokenString)
	if err == nil {
		t.Error("expected error for expired token")
	}
}

func TestJWTProvider_RefreshTokens(t *testing.T) {
	store := newMockUserStore()
	user := &User{
		ID:       "user-123",
		Username: "testuser",
		Role:     "user",
		Status:   "active",
	}
	store.users[user.ID] = user

	config := JWTConfig{
		Secret:        "test-secret-key-min-32-characters",
		Expiry:        3600,
		RefreshExpiry: 86400,
		Issuer:        "test-issuer",
	}
	provider := NewJWTProvider(config, store)

	tokens, _ := provider.GenerateTokens(context.Background(), user)

	// Add small delay to ensure different IssuedAt timestamp
	time.Sleep(time.Second)

	newTokens, err := provider.RefreshTokens(context.Background(), tokens.RefreshToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if newTokens.AccessToken == "" {
		t.Error("new access token should not be empty")
	}

	// Verify the new token is valid
	claims, err := provider.ValidateToken(context.Background(), newTokens.AccessToken)
	if err != nil {
		t.Errorf("new access token should be valid: %v", err)
	}

	if claims.UserID != user.ID {
		t.Errorf("claims should have correct user ID")
	}
}

func TestJWTProvider_Name(t *testing.T) {
	store := newMockUserStore()
	provider := NewJWTProvider(JWTConfig{}, store)

	if provider.Name() != "jwt" {
		t.Errorf("expected name 'jwt', got '%s'", provider.Name())
	}
}

func TestExtractTokenFromHeader(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{
			name:   "valid bearer token",
			header: "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			want:   "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
		},
		{
			name:   "empty header",
			header: "",
			want:   "",
		},
		{
			name:   "no bearer prefix",
			header: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			want:   "",
		},
		{
			name:   "lowercase bearer",
			header: "bearer token",
			want:   "",
		},
		{
			name:   "just Bearer",
			header: "Bearer",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractTokenFromHeader(tt.header)
			if got != tt.want {
				t.Errorf("ExtractTokenFromHeader(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestDefaultJWTConfig(t *testing.T) {
	config := DefaultJWTConfig()

	if config.Expiry != 86400 {
		t.Errorf("expected Expiry 86400, got %d", config.Expiry)
	}

	if config.RefreshExpiry != 604800 {
		t.Errorf("expected RefreshExpiry 604800, got %d", config.RefreshExpiry)
	}

	if config.Issuer != "tugo" {
		t.Errorf("expected Issuer 'tugo', got '%s'", config.Issuer)
	}
}
