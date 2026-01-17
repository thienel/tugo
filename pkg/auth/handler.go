package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/thienel/tugo/pkg/apperror"
	"github.com/thienel/tugo/pkg/response"
	"go.uber.org/zap"
)

// Handler handles authentication HTTP requests.
type Handler struct {
	provider      Provider
	userStore     UserStore
	totpManager   *TOTPManager
	sessionConfig *SessionConfig
	logger        *zap.SugaredLogger
}

// HandlerConfig holds handler configuration.
type HandlerConfig struct {
	Provider      Provider
	UserStore     UserStore
	TOTPManager   *TOTPManager
	SessionConfig *SessionConfig
	Logger        *zap.SugaredLogger
}

// NewHandler creates a new auth handler.
func NewHandler(config HandlerConfig) *Handler {
	return &Handler{
		provider:      config.Provider,
		userStore:     config.UserStore,
		totpManager:   config.TOTPManager,
		sessionConfig: config.SessionConfig,
		logger:        config.Logger,
	}
}

// LoginRequest represents a login request.
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	TOTPCode string `json:"totp_code,omitempty"`
}

// RefreshRequest represents a token refresh request.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Login handles POST /auth/login requests.
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.FromAppError(
			apperror.ErrBadRequest.WithMessage("Invalid request body"),
		))
		return
	}

	// Authenticate user
	user, err := h.provider.Authenticate(c.Request.Context(), Credentials{
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		h.handleError(c, err)
		return
	}

	// Check if TOTP is enabled
	if user.TOTPEnabled {
		if req.TOTPCode == "" {
			c.JSON(http.StatusUnauthorized, response.Error(
				"TOTP_REQUIRED",
				"TOTP code is required",
			))
			return
		}

		// Validate TOTP code
		if h.totpManager != nil {
			if err := h.totpManager.ValidateCodeForUser(c.Request.Context(), user.ID, req.TOTPCode); err != nil {
				h.handleError(c, err)
				return
			}
		}
	}

	// Generate tokens
	tokens, err := h.provider.GenerateTokens(c.Request.Context(), user)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// Set cookie if session-based
	if h.sessionConfig != nil {
		h.setSessionCookie(c, tokens.AccessToken)
	}

	c.JSON(http.StatusOK, response.Success(AuthResponse{
		TokenPair: *tokens,
		User:      user,
	}))
}

// Logout handles POST /auth/logout requests.
func (h *Handler) Logout(c *gin.Context) {
	// Get token from header or cookie
	token := ""
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		token = ExtractTokenFromHeader(authHeader)
	}
	if token == "" && h.sessionConfig != nil {
		token, _ = c.Cookie(h.sessionConfig.CookieName)
	}

	if token != "" {
		// Revoke token
		if err := h.provider.RevokeToken(c.Request.Context(), token); err != nil {
			h.logger.Warnw("Failed to revoke token", "error", err)
		}
	}

	// Clear cookie
	if h.sessionConfig != nil {
		h.clearSessionCookie(c)
	}

	c.JSON(http.StatusOK, response.Success(nil))
}

// Refresh handles POST /auth/refresh requests.
func (h *Handler) Refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.FromAppError(
			apperror.ErrBadRequest.WithMessage("Invalid request body"),
		))
		return
	}

	// Refresh tokens
	tokens, err := h.provider.RefreshTokens(c.Request.Context(), req.RefreshToken)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, response.Success(tokens))
}

// Me handles GET /auth/me requests.
func (h *Handler) Me(c *gin.Context) {
	user := GetUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, response.FromAppError(apperror.ErrUnauthorized))
		return
	}

	c.JSON(http.StatusOK, response.Success(user))
}

// TOTPSetupRequest represents a TOTP setup request.
type TOTPSetupRequest struct {
	Password string `json:"password" binding:"required"`
}

// TOTPSetup handles POST /auth/totp/setup requests.
func (h *Handler) TOTPSetup(c *gin.Context) {
	if h.totpManager == nil {
		c.JSON(http.StatusNotImplemented, response.Error(
			"NOT_IMPLEMENTED",
			"TOTP is not enabled",
		))
		return
	}

	user := GetUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, response.FromAppError(apperror.ErrUnauthorized))
		return
	}

	// Verify password first
	var req TOTPSetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.FromAppError(
			apperror.ErrBadRequest.WithMessage("Invalid request body"),
		))
		return
	}

	// Verify password
	passwordHash, err := h.userStore.GetPasswordHash(c.Request.Context(), user.ID)
	if err != nil {
		h.handleError(c, apperror.ErrInternalServer.WithError(err))
		return
	}

	if !CheckPassword(req.Password, passwordHash) {
		c.JSON(http.StatusUnauthorized, response.FromAppError(apperror.ErrInvalidCredentials))
		return
	}

	// Generate TOTP secret
	setupResponse, err := h.totpManager.SetupTOTP(c.Request.Context(), user.ID, user.Username)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, response.Success(setupResponse))
}

// TOTPEnableRequest represents a TOTP enable request.
type TOTPEnableRequest struct {
	Code string `json:"code" binding:"required"`
}

// TOTPEnable handles POST /auth/totp/enable requests.
func (h *Handler) TOTPEnable(c *gin.Context) {
	if h.totpManager == nil {
		c.JSON(http.StatusNotImplemented, response.Error(
			"NOT_IMPLEMENTED",
			"TOTP is not enabled",
		))
		return
	}

	user := GetUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, response.FromAppError(apperror.ErrUnauthorized))
		return
	}

	var req TOTPEnableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.FromAppError(
			apperror.ErrBadRequest.WithMessage("Invalid request body"),
		))
		return
	}

	// Enable TOTP
	if err := h.totpManager.EnableTOTP(c.Request.Context(), user.ID, req.Code); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, response.Success(map[string]bool{"totp_enabled": true}))
}

// TOTPDisable handles POST /auth/totp/disable requests.
func (h *Handler) TOTPDisable(c *gin.Context) {
	if h.totpManager == nil {
		c.JSON(http.StatusNotImplemented, response.Error(
			"NOT_IMPLEMENTED",
			"TOTP is not enabled",
		))
		return
	}

	user := GetUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, response.FromAppError(apperror.ErrUnauthorized))
		return
	}

	var req TOTPEnableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.FromAppError(
			apperror.ErrBadRequest.WithMessage("Invalid request body"),
		))
		return
	}

	// Disable TOTP
	if err := h.totpManager.DisableTOTP(c.Request.Context(), user.ID, req.Code); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, response.Success(map[string]bool{"totp_enabled": false}))
}

// RegisterRoutes registers auth routes on a Gin router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, authMiddleware gin.HandlerFunc) {
	// Public routes (no auth required)
	rg.POST("/login", h.Login)
	rg.POST("/refresh", h.Refresh)

	// Protected routes (auth required)
	protected := rg.Group("")
	if authMiddleware != nil {
		protected.Use(authMiddleware)
	}
	protected.POST("/logout", h.Logout)
	protected.GET("/me", h.Me)
	protected.POST("/totp/setup", h.TOTPSetup)
	protected.POST("/totp/enable", h.TOTPEnable)
	protected.POST("/totp/disable", h.TOTPDisable)
}

// handleError converts errors to HTTP responses.
func (h *Handler) handleError(c *gin.Context, err error) {
	if appErr, ok := apperror.AsAppError(err); ok {
		c.JSON(appErr.HTTPStatus, response.FromAppError(appErr))
		return
	}

	h.logger.Errorw("Unexpected auth error", "error", err)
	c.JSON(http.StatusInternalServerError, response.FromAppError(apperror.ErrInternalServer))
}

// setSessionCookie sets the session cookie.
func (h *Handler) setSessionCookie(c *gin.Context, token string) {
	if h.sessionConfig == nil {
		return
	}

	c.SetCookie(
		h.sessionConfig.CookieName,
		token,
		h.sessionConfig.MaxAge,
		h.sessionConfig.Path,
		h.sessionConfig.Domain,
		h.sessionConfig.Secure,
		h.sessionConfig.HttpOnly,
	)
}

// clearSessionCookie clears the session cookie.
func (h *Handler) clearSessionCookie(c *gin.Context) {
	if h.sessionConfig == nil {
		return
	}

	c.SetCookie(
		h.sessionConfig.CookieName,
		"",
		-1,
		h.sessionConfig.Path,
		h.sessionConfig.Domain,
		h.sessionConfig.Secure,
		h.sessionConfig.HttpOnly,
	)
}
