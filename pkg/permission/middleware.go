package permission

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/thienel/tugo/pkg/auth"
	"github.com/thienel/tugo/pkg/response"
)

// ContextKey is the type for context keys.
type ContextKey string

const (
	// CheckResultKey is the context key for the permission check result.
	CheckResultKey ContextKey = "tugo_permission_result"
)

// Middleware returns a Gin middleware that checks permissions.
func Middleware(checker *Checker) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user from context
		user, ok := c.Get(string(auth.UserContextKey))
		if !ok {
			response.Unauthorized(c, "authentication required")
			c.Abort()
			return
		}

		authUser, ok := user.(*auth.User)
		if !ok {
			response.Unauthorized(c, "invalid user context")
			c.Abort()
			return
		}

		// Determine action from HTTP method
		action := methodToAction(c.Request.Method)

		// Get collection from route parameter
		collection := c.Param("collection")
		if collection == "" {
			// Try to extract from path
			collection = extractCollectionFromPath(c.Request.URL.Path)
		}

		if collection == "" {
			c.Next()
			return
		}

		// Check permission
		result, err := checker.Check(c.Request.Context(), authUser, collection, action)
		if err != nil {
			response.InternalError(c, "permission check failed")
			c.Abort()
			return
		}

		if !result.Allowed {
			response.Forbidden(c, result.Reason)
			c.Abort()
			return
		}

		// Store result in context for later use
		c.Set(string(CheckResultKey), result)

		c.Next()
	}
}

// RequirePermission returns middleware that checks a specific permission.
func RequirePermission(checker *Checker, collection string, action Action) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user from context
		user, ok := c.Get(string(auth.UserContextKey))
		if !ok {
			response.Unauthorized(c, "authentication required")
			c.Abort()
			return
		}

		authUser, ok := user.(*auth.User)
		if !ok {
			response.Unauthorized(c, "invalid user context")
			c.Abort()
			return
		}

		// Check permission
		result, err := checker.Check(c.Request.Context(), authUser, collection, action)
		if err != nil {
			response.InternalError(c, "permission check failed")
			c.Abort()
			return
		}

		if !result.Allowed {
			response.Forbidden(c, result.Reason)
			c.Abort()
			return
		}

		// Store result in context
		c.Set(string(CheckResultKey), result)

		c.Next()
	}
}

// RequireAction returns middleware that requires a specific action type.
func RequireAction(checker *Checker, action Action) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user from context
		user, ok := c.Get(string(auth.UserContextKey))
		if !ok {
			response.Unauthorized(c, "authentication required")
			c.Abort()
			return
		}

		authUser, ok := user.(*auth.User)
		if !ok {
			response.Unauthorized(c, "invalid user context")
			c.Abort()
			return
		}

		// Get collection from route parameter
		collection := c.Param("collection")
		if collection == "" {
			collection = extractCollectionFromPath(c.Request.URL.Path)
		}

		if collection == "" {
			response.BadRequest(c, "collection not specified")
			c.Abort()
			return
		}

		// Check permission
		result, err := checker.Check(c.Request.Context(), authUser, collection, action)
		if err != nil {
			response.InternalError(c, "permission check failed")
			c.Abort()
			return
		}

		if !result.Allowed {
			response.Forbidden(c, result.Reason)
			c.Abort()
			return
		}

		// Store result in context
		c.Set(string(CheckResultKey), result)

		c.Next()
	}
}

// GetCheckResult retrieves the permission check result from context.
func GetCheckResult(c *gin.Context) *CheckResult {
	if result, ok := c.Get(string(CheckResultKey)); ok {
		if r, ok := result.(*CheckResult); ok {
			return r
		}
	}
	return nil
}

// methodToAction converts HTTP method to permission action.
func methodToAction(method string) Action {
	switch method {
	case http.MethodGet, http.MethodHead:
		return ActionRead
	case http.MethodPost:
		return ActionCreate
	case http.MethodPut, http.MethodPatch:
		return ActionUpdate
	case http.MethodDelete:
		return ActionDelete
	default:
		return ActionRead
	}
}

// extractCollectionFromPath extracts collection name from URL path.
func extractCollectionFromPath(path string) string {
	// Expected patterns:
	// /api/v1/{collection}
	// /api/v1/{collection}/{id}
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// Find "v1" or "api" and take the next segment as collection
	for i, part := range parts {
		if part == "v1" || part == "api" {
			if i+1 < len(parts) && !isReservedPath(parts[i+1]) {
				return parts[i+1]
			}
		}
	}

	// Fallback: return last meaningful segment
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		// If it looks like an ID, return second-to-last
		if len(parts) > 1 && (isUUID(last) || isNumeric(last)) {
			return parts[len(parts)-2]
		}
		if !isReservedPath(last) {
			return last
		}
	}

	return ""
}

// isReservedPath checks if a path segment is reserved.
func isReservedPath(segment string) bool {
	reserved := []string{"auth", "admin", "files", "health", "api", "v1", "v2"}
	for _, r := range reserved {
		if segment == r {
			return true
		}
	}
	return false
}

// isUUID checks if a string looks like a UUID.
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	// Simple check for UUID format: 8-4-4-4-12
	return s[8] == '-' && s[13] == '-' && s[18] == '-' && s[23] == '-'
}

// isNumeric checks if a string is numeric.
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}
