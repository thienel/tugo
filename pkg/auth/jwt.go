package auth

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/thienel/tugo/pkg/apperror"
)

// JWTConfig holds JWT configuration.
type JWTConfig struct {
	// Secret is the signing key for HS256.
	Secret string

	// Expiry is the access token expiry in seconds.
	Expiry int

	// RefreshExpiry is the refresh token expiry in seconds.
	RefreshExpiry int

	// Issuer is the JWT issuer claim.
	Issuer string
}

// DefaultJWTConfig returns default JWT configuration.
func DefaultJWTConfig() JWTConfig {
	return JWTConfig{
		Expiry:        86400,   // 24 hours
		RefreshExpiry: 604800, // 7 days
		Issuer:        "tugo",
	}
}

// JWTClaims represents the JWT claims structure.
type JWTClaims struct {
	jwt.RegisteredClaims
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	RoleID   string `json:"role_id,omitempty"`
	Type     string `json:"type"` // "access" or "refresh"
}

// JWTProvider implements JWT-based authentication.
type JWTProvider struct {
	config    JWTConfig
	userStore UserStore
}

// NewJWTProvider creates a new JWT provider.
func NewJWTProvider(config JWTConfig, userStore UserStore) *JWTProvider {
	if config.Expiry == 0 {
		config.Expiry = DefaultJWTConfig().Expiry
	}
	if config.RefreshExpiry == 0 {
		config.RefreshExpiry = DefaultJWTConfig().RefreshExpiry
	}
	if config.Issuer == "" {
		config.Issuer = DefaultJWTConfig().Issuer
	}

	return &JWTProvider{
		config:    config,
		userStore: userStore,
	}
}

// Name returns the provider name.
func (p *JWTProvider) Name() string {
	return "jwt"
}

// Authenticate validates credentials and returns a user.
func (p *JWTProvider) Authenticate(ctx context.Context, creds Credentials) (*User, error) {
	// Get user by username or email
	var user *User
	var err error

	user, err = p.userStore.GetByUsername(ctx, creds.Username)
	if err != nil {
		// Try by email
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

// GenerateTokens creates access and refresh tokens for a user.
func (p *JWTProvider) GenerateTokens(ctx context.Context, user *User) (*TokenPair, error) {
	now := time.Now()

	// Create access token
	accessClaims := JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    p.config.Issuer,
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(p.config.Expiry) * time.Second)),
		},
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RoleID:   user.RoleID,
		Type:     "access",
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString([]byte(p.config.Secret))
	if err != nil {
		return nil, apperror.ErrInternalServer.WithError(err)
	}

	// Create refresh token
	refreshClaims := JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    p.config.Issuer,
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(p.config.RefreshExpiry) * time.Second)),
		},
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RoleID:   user.RoleID,
		Type:     "refresh",
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString([]byte(p.config.Secret))
	if err != nil {
		return nil, apperror.ErrInternalServer.WithError(err)
	}

	return &TokenPair{
		AccessToken:  accessTokenString,
		RefreshToken: refreshTokenString,
		ExpiresIn:    int64(p.config.Expiry),
		TokenType:    "Bearer",
	}, nil
}

// ValidateToken validates an access token and returns claims.
func (p *JWTProvider) ValidateToken(ctx context.Context, tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(p.config.Secret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, apperror.ErrTokenExpired
		}
		return nil, apperror.ErrUnauthorized.WithError(err)
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, apperror.ErrUnauthorized.WithMessage("Invalid token")
	}

	// Ensure it's an access token
	if claims.Type != "access" {
		return nil, apperror.ErrUnauthorized.WithMessage("Invalid token type")
	}

	return &Claims{
		UserID:   claims.UserID,
		Username: claims.Username,
		Role:     claims.Role,
		RoleID:   claims.RoleID,
	}, nil
}

// RefreshTokens exchanges a refresh token for new tokens.
func (p *JWTProvider) RefreshTokens(ctx context.Context, refreshTokenString string) (*TokenPair, error) {
	token, err := jwt.ParseWithClaims(refreshTokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(p.config.Secret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, apperror.ErrTokenExpired.WithMessage("Refresh token expired")
		}
		return nil, apperror.ErrUnauthorized.WithError(err)
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, apperror.ErrUnauthorized.WithMessage("Invalid refresh token")
	}

	// Ensure it's a refresh token
	if claims.Type != "refresh" {
		return nil, apperror.ErrUnauthorized.WithMessage("Invalid token type")
	}

	// Get fresh user data
	user, err := p.userStore.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, apperror.ErrUnauthorized.WithMessage("User not found")
	}

	// Check if user is still active
	if user.Status != "" && user.Status != "active" {
		return nil, apperror.ErrForbidden.WithMessage("Account is not active")
	}

	// Generate new tokens
	return p.GenerateTokens(ctx, user)
}

// RevokeToken invalidates a token.
// Note: For stateless JWT, we can't truly revoke tokens without a blacklist.
// This is a no-op for basic JWT. Implement token blacklist for production use.
func (p *JWTProvider) RevokeToken(ctx context.Context, token string) error {
	// In a production system, you would add the token to a blacklist
	// stored in Redis or similar fast storage.
	return nil
}

// ExtractTokenFromHeader extracts the JWT token from Authorization header.
func ExtractTokenFromHeader(authHeader string) string {
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		return authHeader[7:]
	}
	return ""
}
