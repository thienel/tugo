package tugo

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/pquerna/otp"
	"github.com/thienel/tlog"
	"github.com/thienel/tugo/pkg/auth"
	"github.com/thienel/tugo/pkg/collection"
	"github.com/thienel/tugo/pkg/schema"
	"go.uber.org/zap"
)

// Engine is the main TuGo engine.
type Engine struct {
	config        Config
	db            *sqlx.DB
	ownsDB        bool
	logger        *zap.SugaredLogger
	router        *gin.Engine
	schemaManager *schema.Manager
	collService   *collection.Service
	collHandler   *collection.Handler

	// Auth components
	authProvider   auth.Provider
	userStore      auth.UserStore
	sessionStore   auth.SessionStore
	totpManager    *auth.TOTPManager
	authHandler    *auth.Handler
	authMiddleware gin.HandlerFunc
}

// New creates a new TuGo engine with the given configuration.
func New(config Config) (*Engine, error) {
	// Merge with defaults
	defaults := DefaultConfig()
	if config.Discovery.Prefix == "" {
		config.Discovery.Prefix = defaults.Discovery.Prefix
	}
	if config.Discovery.Mode == "" {
		config.Discovery.Mode = defaults.Discovery.Mode
	}
	if config.Server.Port == 0 {
		config.Server.Port = defaults.Server.Port
	}

	// Initialize logger
	_ = tlog.InitWithDefaults()
	logger := tlog.S()

	// Initialize database connection
	var db *sqlx.DB
	var ownsDB bool
	var err error

	if config.DB != nil {
		db = config.DB
		ownsDB = false
	} else if config.DatabaseURL != "" {
		db, err = sqlx.Connect("postgres", config.DatabaseURL)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to database: %w", err)
		}
		ownsDB = true

		// Configure connection pool
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(5 * time.Minute)
	} else {
		return nil, fmt.Errorf("either DB or DatabaseURL must be provided")
	}

	// Create schema manager config
	schemaConfig := schema.ManagerConfig{
		Mode:         schema.DiscoveryMode(config.Discovery.Mode),
		Prefix:       config.Discovery.Prefix,
		AutoDiscover: config.Discovery.AutoDiscover,
		Blacklist:    config.Discovery.Blacklist,
		Config:       make(map[string]schema.CollectionConfig),
	}

	// Convert collection configs
	for name, cfg := range config.Discovery.Config {
		schemaConfig.Config[name] = schema.CollectionConfig{
			Enabled:      cfg.Enabled,
			PublicFields: cfg.PublicFields,
		}
	}

	// Create schema manager
	schemaManager := schema.NewManager(db, schemaConfig, logger)

	// Create repository and service
	repo := collection.NewRepository(db)
	collService := collection.NewService(repo, schemaManager, logger)
	collHandler := collection.NewHandler(collService, logger)

	// Create Gin router
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	engine := &Engine{
		config:        config,
		db:            db,
		ownsDB:        ownsDB,
		logger:        logger,
		router:        router,
		schemaManager: schemaManager,
		collService:   collService,
		collHandler:   collHandler,
	}

	// Initialize authentication if configured
	if len(config.Auth.Methods) > 0 {
		if err := engine.initAuth(); err != nil {
			return nil, fmt.Errorf("failed to initialize auth: %w", err)
		}
	}

	return engine, nil
}

// initAuth initializes authentication components.
func (e *Engine) initAuth() error {
	// Create user store
	e.userStore = auth.NewDBUserStore(e.db, "tugo_users")

	// Create session store (for session-based auth)
	e.sessionStore = auth.NewDBSessionStore(e.db, "tugo_sessions")

	// Determine primary auth method
	primaryMethod := "jwt"
	if len(e.config.Auth.Methods) > 0 {
		primaryMethod = e.config.Auth.Methods[0]
	}

	// Create auth provider based on configuration
	switch primaryMethod {
	case "jwt":
		jwtConfig := auth.JWTConfig{
			Secret:        e.config.Auth.JWT.Secret,
			Expiry:        e.config.Auth.JWT.Expiry,
			RefreshExpiry: e.config.Auth.JWT.RefreshExp,
			Issuer:        e.config.Auth.JWT.Issuer,
		}
		e.authProvider = auth.NewJWTProvider(jwtConfig, e.userStore)

	case "cookie", "session":
		sessionConfig := auth.SessionConfig{
			CookieName: e.config.Auth.Cookie.Name,
			MaxAge:     e.config.Auth.Cookie.MaxAge,
			Secure:     e.config.Auth.Cookie.Secure,
			HttpOnly:   e.config.Auth.Cookie.HttpOnly,
			SameSite:   e.config.Auth.Cookie.SameSite,
		}
		e.authProvider = auth.NewSessionProvider(sessionConfig, e.userStore, e.sessionStore)

	default:
		// Default to JWT
		e.authProvider = auth.NewJWTProvider(auth.DefaultJWTConfig(), e.userStore)
	}

	// Create TOTP manager if enabled
	for _, method := range e.config.Auth.Methods {
		if method == "totp" {
			totpConfig := auth.TOTPConfig{
				Issuer: e.config.Auth.TOTP.Issuer,
				Period: uint(e.config.Auth.TOTP.Period),
				Digits: otp.Digits(e.config.Auth.TOTP.Digits),
			}
			e.totpManager = auth.NewTOTPManager(totpConfig, e.userStore)
			break
		}
	}

	// Create session config for auth handler (if using cookies)
	var sessionConfigPtr *auth.SessionConfig
	for _, method := range e.config.Auth.Methods {
		if method == "cookie" || method == "session" {
			sessionConfig := auth.SessionConfig{
				CookieName: e.config.Auth.Cookie.Name,
				MaxAge:     e.config.Auth.Cookie.MaxAge,
				Secure:     e.config.Auth.Cookie.Secure,
				HttpOnly:   e.config.Auth.Cookie.HttpOnly,
				SameSite:   e.config.Auth.Cookie.SameSite,
			}
			sessionConfigPtr = &sessionConfig
			break
		}
	}

	// Create auth handler
	e.authHandler = auth.NewHandler(auth.HandlerConfig{
		Provider:      e.authProvider,
		UserStore:     e.userStore,
		TOTPManager:   e.totpManager,
		SessionConfig: sessionConfigPtr,
		Logger:        e.logger,
	})

	// Create auth middleware
	e.authMiddleware = auth.RequireAuth(e.authProvider, e.userStore)

	e.logger.Infow("Authentication initialized", "methods", e.config.Auth.Methods)

	return nil
}

// Init initializes the engine by discovering the schema.
func (e *Engine) Init(ctx context.Context) error {
	e.logger.Info("Initializing TuGo engine...")

	// Discover schema
	if err := e.schemaManager.Refresh(ctx); err != nil {
		return fmt.Errorf("failed to refresh schema: %w", err)
	}

	// Log discovered collections
	collections := e.schemaManager.GetCollections()
	e.logger.Infow("Discovered collections", "count", len(collections))
	for _, c := range collections {
		e.logger.Debugw("Collection", "name", c.Name, "table", c.TableName, "fields", len(c.Fields))
	}

	return nil
}

// Mount mounts the TuGo API routes to a Gin router group.
// This is the primary use case for middleware integration.
func (e *Engine) Mount(rg *gin.RouterGroup) {
	// Mount auth routes if enabled
	if e.authHandler != nil {
		authGroup := rg.Group("/auth")
		e.authHandler.RegisterRoutes(authGroup, e.authMiddleware)
		e.logger.Infow("Auth routes mounted", "path", authGroup.BasePath())
	}

	// Mount collection routes
	e.collHandler.RegisterRoutes(rg)

	e.logger.Infow("TuGo routes mounted", "path", rg.BasePath())
}

// MountWithAuth mounts routes with authentication middleware.
func (e *Engine) MountWithAuth(rg *gin.RouterGroup) {
	// Mount auth routes if enabled
	if e.authHandler != nil {
		authGroup := rg.Group("/auth")
		e.authHandler.RegisterRoutes(authGroup, e.authMiddleware)
	}

	// Apply auth middleware to collection routes
	if e.authMiddleware != nil {
		rg.Use(e.authMiddleware)
	}

	// Mount collection routes
	e.collHandler.RegisterRoutes(rg)

	e.logger.Infow("TuGo routes mounted with auth", "path", rg.BasePath())
}

// Router returns the internal Gin router for standalone mode.
func (e *Engine) Router() *gin.Engine {
	return e.router
}

// Run starts the HTTP server in standalone mode.
func (e *Engine) Run(addr string) error {
	if addr == "" {
		addr = fmt.Sprintf(":%d", e.config.Server.Port)
	}

	// Mount routes on /api/v1
	v1 := e.router.Group("/api/v1")
	e.Mount(v1)

	e.logger.Infow("Starting TuGo server", "address", addr)

	server := &http.Server{
		Addr:         addr,
		Handler:      e.router,
		ReadTimeout:  e.config.Server.ReadTimeout,
		WriteTimeout: e.config.Server.WriteTimeout,
	}

	return server.ListenAndServe()
}

// Close cleans up resources.
func (e *Engine) Close() error {
	if e.ownsDB && e.db != nil {
		return e.db.Close()
	}
	return nil
}

// DB returns the database connection.
func (e *Engine) DB() *sqlx.DB {
	return e.db
}

// SchemaManager returns the schema manager.
func (e *Engine) SchemaManager() *schema.Manager {
	return e.schemaManager
}

// RefreshSchema re-discovers the database schema.
func (e *Engine) RefreshSchema(ctx context.Context) error {
	return e.schemaManager.Refresh(ctx)
}

// GetCollections returns all discovered collections.
func (e *Engine) GetCollections() []*schema.Collection {
	return e.schemaManager.GetCollections()
}

// HasCollection checks if a collection exists.
func (e *Engine) HasCollection(name string) bool {
	return e.schemaManager.HasCollection(name)
}

// AuthProvider returns the auth provider.
func (e *Engine) AuthProvider() auth.Provider {
	return e.authProvider
}

// AuthMiddleware returns the auth middleware.
func (e *Engine) AuthMiddleware() gin.HandlerFunc {
	return e.authMiddleware
}

// UserStore returns the user store.
func (e *Engine) UserStore() auth.UserStore {
	return e.userStore
}

// TOTPManager returns the TOTP manager.
func (e *Engine) TOTPManager() *auth.TOTPManager {
	return e.totpManager
}
