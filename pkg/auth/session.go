package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/thienel/tugo/pkg/apperror"
)

// SessionConfig holds session configuration.
type SessionConfig struct {
	// CookieName is the session cookie name.
	CookieName string

	// MaxAge is the session max age in seconds.
	MaxAge int

	// Secure sets the Secure flag on cookies.
	Secure bool

	// HttpOnly sets the HttpOnly flag on cookies.
	HttpOnly bool

	// SameSite sets the SameSite attribute.
	SameSite string

	// Domain sets the cookie domain.
	Domain string

	// Path sets the cookie path.
	Path string
}

// DefaultSessionConfig returns default session configuration.
func DefaultSessionConfig() SessionConfig {
	return SessionConfig{
		CookieName: "tugo_session",
		MaxAge:     86400, // 24 hours
		Secure:     true,
		HttpOnly:   true,
		SameSite:   "Lax",
		Path:       "/",
	}
}

// SessionProvider implements session-based authentication.
type SessionProvider struct {
	config       SessionConfig
	userStore    UserStore
	sessionStore SessionStore
}

// NewSessionProvider creates a new session provider.
func NewSessionProvider(config SessionConfig, userStore UserStore, sessionStore SessionStore) *SessionProvider {
	if config.CookieName == "" {
		config.CookieName = DefaultSessionConfig().CookieName
	}
	if config.MaxAge == 0 {
		config.MaxAge = DefaultSessionConfig().MaxAge
	}
	if config.Path == "" {
		config.Path = DefaultSessionConfig().Path
	}

	return &SessionProvider{
		config:       config,
		userStore:    userStore,
		sessionStore: sessionStore,
	}
}

// Name returns the provider name.
func (p *SessionProvider) Name() string {
	return "session"
}

// Authenticate validates credentials and returns a user.
func (p *SessionProvider) Authenticate(ctx context.Context, creds Credentials) (*User, error) {
	// Get user by username or email
	var user *User
	var err error

	user, err = p.userStore.GetByUsername(ctx, creds.Username)
	if err != nil {
		user, err = p.userStore.GetByEmail(ctx, creds.Username)
		if err != nil {
			return nil, apperror.ErrInvalidCredentials
		}
	}

	// Check if user is active
	if user.Status != "" && user.Status != "active" {
		return nil, apperror.ErrForbidden.WithMessage("Account is not active")
	}

	// Verify password
	passwordHash, err := p.userStore.GetPasswordHash(ctx, user.ID)
	if err != nil {
		return nil, apperror.ErrInternalServer.WithError(err)
	}

	if !CheckPassword(creds.Password, passwordHash) {
		return nil, apperror.ErrInvalidCredentials
	}

	return user, nil
}

// GenerateTokens creates a session token for a user.
func (p *SessionProvider) GenerateTokens(ctx context.Context, user *User) (*TokenPair, error) {
	// Generate random session token
	token, err := generateSecureToken(32)
	if err != nil {
		return nil, apperror.ErrInternalServer.WithError(err)
	}

	// Create session
	session := &Session{
		ID:        generateID(),
		UserID:    user.ID,
		Token:     token,
		ExpiresAt: time.Now().Add(time.Duration(p.config.MaxAge) * time.Second),
		CreatedAt: time.Now(),
	}

	if err := p.sessionStore.Create(ctx, session); err != nil {
		return nil, apperror.ErrInternalServer.WithError(err)
	}

	return &TokenPair{
		AccessToken: token,
		ExpiresIn:   int64(p.config.MaxAge),
		TokenType:   "Session",
	}, nil
}

// ValidateToken validates a session token and returns claims.
func (p *SessionProvider) ValidateToken(ctx context.Context, token string) (*Claims, error) {
	session, err := p.sessionStore.GetByToken(ctx, token)
	if err != nil {
		return nil, apperror.ErrUnauthorized.WithMessage("Invalid session")
	}

	// Check if session is expired
	if time.Now().After(session.ExpiresAt) {
		// Clean up expired session
		_ = p.sessionStore.Delete(ctx, token)
		return nil, apperror.ErrTokenExpired.WithMessage("Session expired")
	}

	// Get user
	user, err := p.userStore.GetByID(ctx, session.UserID)
	if err != nil {
		return nil, apperror.ErrUnauthorized.WithMessage("User not found")
	}

	// Check if user is still active
	if user.Status != "" && user.Status != "active" {
		return nil, apperror.ErrForbidden.WithMessage("Account is not active")
	}

	return &Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RoleID:   user.RoleID,
	}, nil
}

// RefreshTokens is not applicable for session-based auth.
func (p *SessionProvider) RefreshTokens(ctx context.Context, refreshToken string) (*TokenPair, error) {
	return nil, apperror.ErrBadRequest.WithMessage("Session-based auth does not support refresh tokens")
}

// RevokeToken invalidates a session.
func (p *SessionProvider) RevokeToken(ctx context.Context, token string) error {
	return p.sessionStore.Delete(ctx, token)
}

// RevokeAllUserSessions invalidates all sessions for a user.
func (p *SessionProvider) RevokeAllUserSessions(ctx context.Context, userID string) error {
	return p.sessionStore.DeleteByUserID(ctx, userID)
}

// Config returns the session configuration.
func (p *SessionProvider) Config() SessionConfig {
	return p.config
}

// generateSecureToken generates a cryptographically secure random token.
func generateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// generateID generates a unique ID.
func generateID() string {
	token, _ := generateSecureToken(16)
	return token
}
