package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/thienel/tugo/pkg/apperror"
	"github.com/thienel/tugo/pkg/response"
)

// MiddlewareConfig holds middleware configuration.
type MiddlewareConfig struct {
	// Provider is the authentication provider to use.
	Provider Provider

	// UserStore is the user store for loading user details.
	UserStore UserStore

	// SessionConfig is used for cookie-based auth.
	SessionConfig *SessionConfig

	// SkipPaths are paths that don't require authentication.
	SkipPaths []string

	// Optional determines if authentication is optional.
	// If true, unauthenticated requests will proceed with nil user.
	Optional bool
}

// Middleware creates a Gin middleware for authentication.
func Middleware(config MiddlewareConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if path should be skipped
		if shouldSkipPath(c.Request.URL.Path, config.SkipPaths) {
			c.Next()
			return
		}

		var claims *Claims
		var err error

		// Try to extract token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			token := ExtractTokenFromHeader(authHeader)
			if token != "" {
				claims, err = config.Provider.ValidateToken(c.Request.Context(), token)
			}
		}

		// If no Authorization header, try cookie (for session-based auth)
		if claims == nil && config.SessionConfig != nil {
			if cookie, cookieErr := c.Cookie(config.SessionConfig.CookieName); cookieErr == nil && cookie != "" {
				claims, err = config.Provider.ValidateToken(c.Request.Context(), cookie)
			}
		}

		// Handle authentication result
		if claims == nil {
			if config.Optional {
				// Optional auth - proceed without user
				c.Next()
				return
			}

			// Required auth - return error
			if err != nil {
				if appErr, ok := apperror.AsAppError(err); ok {
					c.AbortWithStatusJSON(appErr.HTTPStatus, response.FromAppError(appErr))
					return
				}
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.FromAppError(apperror.ErrUnauthorized))
			return
		}

		// Load user from store
		user, err := config.UserStore.GetByID(c.Request.Context(), claims.UserID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.FromAppError(
				apperror.ErrUnauthorized.WithMessage("User not found"),
			))
			return
		}

		// Check if user is active
		if user.Status != "" && user.Status != "active" {
			c.AbortWithStatusJSON(http.StatusForbidden, response.FromAppError(
				apperror.ErrForbidden.WithMessage("Account is not active"),
			))
			return
		}

		// Set user and claims in context
		ctx := SetUserInContext(c.Request.Context(), user)
		ctx = SetClaimsInContext(ctx, claims)
		c.Request = c.Request.WithContext(ctx)

		// Also set in Gin context for convenience
		c.Set("user", user)
		c.Set("claims", claims)

		c.Next()
	}
}

// RequireAuth creates a middleware that requires authentication.
func RequireAuth(provider Provider, userStore UserStore) gin.HandlerFunc {
	return Middleware(MiddlewareConfig{
		Provider:  provider,
		UserStore: userStore,
		Optional:  false,
	})
}

// OptionalAuth creates a middleware that allows optional authentication.
func OptionalAuth(provider Provider, userStore UserStore) gin.HandlerFunc {
	return Middleware(MiddlewareConfig{
		Provider:  provider,
		UserStore: userStore,
		Optional:  true,
	})
}

// RequireRole creates a middleware that requires a specific role.
func RequireRole(roles ...string) gin.HandlerFunc {
	roleSet := make(map[string]bool)
	for _, r := range roles {
		roleSet[strings.ToLower(r)] = true
	}

	return func(c *gin.Context) {
		user, exists := c.Get("user")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.FromAppError(apperror.ErrUnauthorized))
			return
		}

		u, ok := user.(*User)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.FromAppError(apperror.ErrUnauthorized))
			return
		}

		if !roleSet[strings.ToLower(u.Role)] {
			c.AbortWithStatusJSON(http.StatusForbidden, response.FromAppError(
				apperror.ErrForbidden.WithMessage("Insufficient permissions"),
			))
			return
		}

		c.Next()
	}
}

// RequireAdmin creates a middleware that requires admin role.
func RequireAdmin() gin.HandlerFunc {
	return RequireRole("admin")
}

// GetUser retrieves the authenticated user from Gin context.
func GetUser(c *gin.Context) *User {
	if user, exists := c.Get("user"); exists {
		if u, ok := user.(*User); ok {
			return u
		}
	}
	return nil
}

// GetClaims retrieves the claims from Gin context.
func GetClaims(c *gin.Context) *Claims {
	if claims, exists := c.Get("claims"); exists {
		if cl, ok := claims.(*Claims); ok {
			return cl
		}
	}
	return nil
}

// shouldSkipPath checks if a path should skip authentication.
func shouldSkipPath(path string, skipPaths []string) bool {
	for _, skip := range skipPaths {
		if matchPath(path, skip) {
			return true
		}
	}
	return false
}

// matchPath checks if a path matches a pattern.
// Supports wildcards: /api/v1/* matches /api/v1/anything
func matchPath(path, pattern string) bool {
	if pattern == path {
		return true
	}

	// Handle wildcard
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(path, prefix)
	}

	return false
}
