package auth

import (
	"context"
	"time"
)

// User represents an authenticated user.
type User struct {
	ID          string         `json:"id"`
	Username    string         `json:"username"`
	Email       string         `json:"email,omitempty"`
	Role        string         `json:"role"`
	RoleID      string         `json:"role_id,omitempty"`
	Status      string         `json:"status,omitempty"`
	TOTPEnabled bool           `json:"totp_enabled,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at,omitempty"`
	UpdatedAt   time.Time      `json:"updated_at,omitempty"`
}

// Credentials represents login credentials.
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TOTPCode string `json:"totp_code,omitempty"`
}

// TokenPair represents access and refresh tokens.
type TokenPair struct {
	AccessToken  string `json:"token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// AuthResponse is the response from login/refresh operations.
type AuthResponse struct {
	TokenPair
	User *User `json:"user"`
}

// Claims represents JWT claims.
type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	RoleID   string `json:"role_id,omitempty"`
}

// Session represents a session stored in database or cookie.
type Session struct {
	ID        string    `json:"id" db:"id"`
	UserID    string    `json:"user_id" db:"user_id"`
	Token     string    `json:"token" db:"token"`
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UserAgent string    `json:"user_agent,omitempty" db:"user_agent"`
	IPAddress string    `json:"ip_address,omitempty" db:"ip_address"`
}

// contextKey is the type for context keys.
type contextKey string

const (
	// UserContextKey is the context key for the authenticated user.
	UserContextKey contextKey = "tugo_user"
	// ClaimsContextKey is the context key for JWT claims.
	ClaimsContextKey contextKey = "tugo_claims"
)

// GetUserFromContext retrieves the user from context.
func GetUserFromContext(ctx context.Context) (*User, bool) {
	user, ok := ctx.Value(UserContextKey).(*User)
	return user, ok
}

// SetUserInContext sets the user in context.
func SetUserInContext(ctx context.Context, user *User) context.Context {
	return context.WithValue(ctx, UserContextKey, user)
}

// GetClaimsFromContext retrieves the claims from context.
func GetClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(ClaimsContextKey).(*Claims)
	return claims, ok
}

// SetClaimsInContext sets the claims in context.
func SetClaimsInContext(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, ClaimsContextKey, claims)
}

// Provider defines the interface for authentication providers.
type Provider interface {
	// Name returns the provider name.
	Name() string

	// Authenticate validates credentials and returns a user.
	Authenticate(ctx context.Context, creds Credentials) (*User, error)

	// GenerateTokens creates access and refresh tokens for a user.
	GenerateTokens(ctx context.Context, user *User) (*TokenPair, error)

	// ValidateToken validates an access token and returns claims.
	ValidateToken(ctx context.Context, token string) (*Claims, error)

	// RefreshTokens exchanges a refresh token for new tokens.
	RefreshTokens(ctx context.Context, refreshToken string) (*TokenPair, error)

	// RevokeToken invalidates a token.
	RevokeToken(ctx context.Context, token string) error
}

// UserStore defines the interface for user storage operations.
type UserStore interface {
	// GetByID retrieves a user by ID.
	GetByID(ctx context.Context, id string) (*User, error)

	// GetByUsername retrieves a user by username.
	GetByUsername(ctx context.Context, username string) (*User, error)

	// GetByEmail retrieves a user by email.
	GetByEmail(ctx context.Context, email string) (*User, error)

	// GetPasswordHash retrieves the password hash for a user.
	GetPasswordHash(ctx context.Context, userID string) (string, error)

	// GetTOTPSecret retrieves the TOTP secret for a user.
	GetTOTPSecret(ctx context.Context, userID string) (string, error)

	// Create creates a new user.
	Create(ctx context.Context, user *User, passwordHash string) error

	// UpdatePassword updates a user's password.
	UpdatePassword(ctx context.Context, userID string, passwordHash string) error

	// SetTOTPSecret sets the TOTP secret for a user.
	SetTOTPSecret(ctx context.Context, userID string, secret string) error

	// EnableTOTP enables TOTP for a user.
	EnableTOTP(ctx context.Context, userID string, enabled bool) error
}

// SessionStore defines the interface for session storage.
type SessionStore interface {
	// Create creates a new session.
	Create(ctx context.Context, session *Session) error

	// GetByToken retrieves a session by token.
	GetByToken(ctx context.Context, token string) (*Session, error)

	// Delete deletes a session.
	Delete(ctx context.Context, token string) error

	// DeleteByUserID deletes all sessions for a user.
	DeleteByUserID(ctx context.Context, userID string) error

	// CleanExpired removes expired sessions.
	CleanExpired(ctx context.Context) error
}
