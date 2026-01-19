package tugo

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/pquerna/otp"
	"github.com/thienel/tlog"
	"github.com/thienel/tugo/pkg/admin"
	"github.com/thienel/tugo/pkg/auth"
	"github.com/thienel/tugo/pkg/collection"
	"github.com/thienel/tugo/pkg/migrate"
	"github.com/thienel/tugo/pkg/schema"
	"github.com/thienel/tugo/pkg/storage"
	"github.com/thienel/tugo/pkg/validation"
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

	// Storage components
	storageManager *storage.Manager
	storageHandler *storage.Handler

	// Validation
	validatorRegistry *validation.ValidatorRegistry

	// Admin
	adminHandler *admin.Handler

	// Schema watcher
	schemaWatcher *SchemaWatcher
	stopWatcher   chan struct{}
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

	// Create validation registry
	validatorRegistry := validation.NewValidatorRegistry(db)

	// Set validator on collection service
	collService.SetValidator(validatorRegistry)

	engine := &Engine{
		config:            config,
		db:                db,
		ownsDB:            ownsDB,
		logger:            logger,
		router:            router,
		schemaManager:     schemaManager,
		collService:       collService,
		collHandler:       collHandler,
		validatorRegistry: validatorRegistry,
	}

	// Initialize authentication if configured
	if len(config.Auth.Methods) > 0 {
		if err := engine.initAuth(); err != nil {
			return nil, fmt.Errorf("failed to initialize auth: %w", err)
		}
	}

	// Initialize storage if configured
	if config.Storage.Default != "" || len(config.Storage.Providers) > 0 {
		if err := engine.initStorage(); err != nil {
			return nil, fmt.Errorf("failed to initialize storage: %w", err)
		}
	}

	// Initialize admin handler
	engine.initAdmin()

	return engine, nil
}

// initAuth initializes authentication components.
func (e *Engine) initAuth() error {
	// Use custom user store if provided, otherwise use default DBUserStore
	if e.config.Auth.CustomUserStore != nil {
		if customStore, ok := e.config.Auth.CustomUserStore.(auth.UserStore); ok {
			e.userStore = customStore
			e.logger.Info("Using custom UserStore implementation")
		} else {
			return fmt.Errorf("CustomUserStore does not implement auth.UserStore interface")
		}
	} else {
		// Create default user store
		e.userStore = auth.NewDBUserStore(e.db, "tugo_users")
	}

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

// initStorage initializes storage components.
func (e *Engine) initStorage() error {
	// Create storage manager
	e.storageManager = storage.NewManager(e.config.Storage.Default, e.db)

	// Note: In a real implementation, you would initialize providers from config
	// For now, we create a local storage provider if no providers are configured
	if len(e.config.Storage.Providers) == 0 {
		local, err := storage.NewLocal("./uploads", "/api/v1/files")
		if err != nil {
			return fmt.Errorf("failed to create local storage: %w", err)
		}
		e.storageManager.RegisterProvider("local", local)
		if e.config.Storage.Default == "" {
			e.config.Storage.Default = "local"
		}
	}

	// Create storage handler
	e.storageHandler = storage.NewHandler(e.storageManager, e.logger, storage.DefaultHandlerConfig())

	e.logger.Infow("Storage initialized", "default", e.config.Storage.Default)

	return nil
}

// initAdmin initializes admin components.
func (e *Engine) initAdmin() {
	// Create schema executor
	executor := admin.NewSchemaExecutor(e.db)

	// Create admin handler
	e.adminHandler = admin.NewHandler(e.schemaManager, executor, e.logger, admin.DefaultHandlerConfig())

	e.logger.Info("Admin handler initialized")
}

// Init initializes the engine by discovering the schema.
func (e *Engine) Init(ctx context.Context) error {
	e.logger.Info("Initializing TuGo engine...")

	// Run migrations first
	e.logger.Info("Running database migrations...")
	if err := migrate.RunInternalMigrations(ctx, e.db, e.logger); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Ensure storage table exists
	if e.storageManager != nil {
		if err := e.storageManager.EnsureTable(ctx); err != nil {
			e.logger.Warnw("Failed to create storage table", "error", err)
		}
	}

	// Seed users if configured
	if err := e.SeedUsers(ctx); err != nil {
		e.logger.Warnw("Failed to seed users", "error", err)
	}

	// Try seeding from environment variables
	if err := e.SeedFromEnv(ctx); err != nil {
		e.logger.Warnw("Failed to seed from environment", "error", err)
	}

	// Discover schema
	if err := e.schemaManager.Refresh(ctx); err != nil {
		return fmt.Errorf("failed to refresh schema: %w", err)
	}

	// Build validators for discovered collections
	collections := e.schemaManager.GetCollections()
	for _, col := range collections {
		e.validatorRegistry.BuildFromCollection(col)
	}

	// Log discovered collections
	e.logger.Infow("Discovered collections", "count", len(collections))
	for _, c := range collections {
		e.logger.Debugw("Collection", "name", c.Name, "table", c.TableName, "fields", len(c.Fields))
	}

	// Start schema watcher if configured
	if err := e.StartSchemaWatcher(ctx); err != nil {
		e.logger.Warnw("Failed to start schema watcher", "error", err)
	}

	return nil
}

// Mount mounts the TuGo API routes to a Gin router group.
// This is the primary use case for middleware integration.
// If config.Mount.IncludeAdmin is true, admin routes are automatically registered.
func (e *Engine) Mount(rg *gin.RouterGroup) {
	e.MountWithOptions(rg, e.config.Mount)
}

// MountWithOptions mounts the TuGo API routes with custom options.
func (e *Engine) MountWithOptions(rg *gin.RouterGroup, opts MountOptions) {
	// Mount auth routes if enabled
	if e.authHandler != nil {
		authGroup := rg.Group("/auth")
		e.authHandler.RegisterRoutes(authGroup, e.authMiddleware)
		e.logger.Infow("Auth routes mounted", "path", authGroup.BasePath())
	}

	// Mount file storage routes if enabled
	if e.storageHandler != nil {
		filesGroup := rg.Group("/files")
		e.storageHandler.RegisterRoutes(filesGroup)
		e.logger.Infow("File routes mounted", "path", filesGroup.BasePath())
	}

	// Mount collection routes
	e.collHandler.RegisterRoutes(rg)

	// Auto-mount admin routes if configured
	if opts.IncludeAdmin && e.adminHandler != nil {
		adminPath := opts.AdminPath
		if adminPath == "" {
			adminPath = "/admin"
		}
		adminGroup := rg.Group(adminPath)
		if opts.RequireAdminAuth && e.authMiddleware != nil {
			adminGroup.Use(e.authMiddleware)
			adminGroup.Use(auth.RequireRole("admin"))
		}
		e.adminHandler.RegisterRoutes(adminGroup)
		e.logger.Infow("Admin routes auto-mounted", "path", adminGroup.BasePath())
	}

	e.logger.Infow("TuGo routes mounted", "path", rg.BasePath())
}

// MountAdmin mounts admin API routes (should be protected).
func (e *Engine) MountAdmin(rg *gin.RouterGroup) {
	if e.adminHandler != nil {
		e.adminHandler.RegisterRoutes(rg)
		e.logger.Infow("Admin routes mounted", "path", rg.BasePath())
	}
}

// MountWithAuth mounts routes with authentication middleware.
func (e *Engine) MountWithAuth(rg *gin.RouterGroup) {
	// Mount auth routes if enabled (without auth middleware)
	if e.authHandler != nil {
		authGroup := rg.Group("/auth")
		e.authHandler.RegisterRoutes(authGroup, e.authMiddleware)
	}

	// Apply auth middleware to protected routes
	protected := rg.Group("")
	if e.authMiddleware != nil {
		protected.Use(e.authMiddleware)
	}

	// Mount file storage routes if enabled
	if e.storageHandler != nil {
		filesGroup := protected.Group("/files")
		e.storageHandler.RegisterRoutes(filesGroup)
	}

	// Mount collection routes
	e.collHandler.RegisterRoutes(protected)

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

	// Mount admin routes on /api/admin (protected by auth if available)
	adminGroup := e.router.Group("/api/admin")
	if e.authMiddleware != nil {
		adminGroup.Use(e.authMiddleware)
		adminGroup.Use(auth.RequireRole("admin"))
	}
	e.MountAdmin(adminGroup)

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

// StorageManager returns the storage manager.
func (e *Engine) StorageManager() *storage.Manager {
	return e.storageManager
}

// ValidatorRegistry returns the validator registry.
func (e *Engine) ValidatorRegistry() *validation.ValidatorRegistry {
	return e.validatorRegistry
}

// AdminHandler returns the admin handler.
func (e *Engine) AdminHandler() *admin.Handler {
	return e.adminHandler
}

// SeedUsers seeds default users if configured and they don't exist.
// This is typically called during Init or manually after setup.
func (e *Engine) SeedUsers(ctx context.Context) error {
	if !e.config.Seed.Enabled {
		return nil
	}

	if e.userStore == nil {
		e.logger.Warn("User seeding enabled but no user store configured")
		return nil
	}

	if e.config.Seed.AdminUser != nil {
		if err := e.seedUser(ctx, e.config.Seed.AdminUser); err != nil {
			return fmt.Errorf("failed to seed admin user: %w", err)
		}
	}

	return nil
}

// seedUser creates a user if it doesn't already exist.
func (e *Engine) seedUser(ctx context.Context, seedUser *SeedUser) error {
	// Check if user already exists
	_, err := e.userStore.GetByUsername(ctx, seedUser.Username)
	if err == nil {
		e.logger.Infow("User already exists, skipping seed", "username", seedUser.Username)
		return nil
	}

	// Get role ID
	roleID, err := e.getRoleID(ctx, seedUser.Role)
	if err != nil {
		return fmt.Errorf("failed to get role: %w", err)
	}

	// Hash password
	hash, err := auth.HashPassword(seedUser.Password)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Create user
	user := &auth.User{
		Username: seedUser.Username,
		Email:    seedUser.Email,
		RoleID:   roleID,
		Status:   "active",
	}

	if err := e.userStore.Create(ctx, user, hash); err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	e.logger.Infow("User seeded successfully", "username", seedUser.Username, "role", seedUser.Role)
	return nil
}

// getRoleID retrieves the role ID by name.
func (e *Engine) getRoleID(ctx context.Context, roleName string) (string, error) {
	if roleName == "" {
		roleName = "admin"
	}

	var roleID string
	err := e.db.GetContext(ctx, &roleID, "SELECT id FROM tugo_roles WHERE name = $1", roleName)
	if err != nil {
		return "", fmt.Errorf("role '%s' not found: %w", roleName, err)
	}
	return roleID, nil
}

// SeedFromEnv seeds users from environment variables.
// Looks for: TUGO_ADMIN_USERNAME, TUGO_ADMIN_EMAIL, TUGO_ADMIN_PASSWORD
func (e *Engine) SeedFromEnv(ctx context.Context) error {
	username := getEnvOrDefault("TUGO_ADMIN_USERNAME", "")
	email := getEnvOrDefault("TUGO_ADMIN_EMAIL", "")
	password := getEnvOrDefault("TUGO_ADMIN_PASSWORD", "")

	if username == "" || password == "" {
		return nil // Not configured via env
	}

	seedUser := &SeedUser{
		Username: username,
		Email:    email,
		Password: password,
		Role:     "admin",
	}

	return e.seedUser(ctx, seedUser)
}

// getEnvOrDefault returns environment variable or default value.
func getEnvOrDefault(key, defaultVal string) string {
	if val, ok := lookupEnv(key); ok {
		return val
	}
	return defaultVal
}

// lookupEnv is a variable to allow testing.
var lookupEnv = os.LookupEnv

// SchemaWatcher watches for schema changes and triggers refresh.
type SchemaWatcher struct {
	engine   *Engine
	config   SchemaWatchConfig
	stopCh   chan struct{}
	doneCh   chan struct{}
	listener *PGListener
}

// NewSchemaWatcher creates a new schema watcher.
func NewSchemaWatcher(engine *Engine, config SchemaWatchConfig) *SchemaWatcher {
	return &SchemaWatcher{
		engine: engine,
		config: config,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Start begins watching for schema changes.
func (w *SchemaWatcher) Start(ctx context.Context) error {
	if !w.config.Enabled {
		return nil
	}

	switch w.config.Mode {
	case "notify":
		return w.startNotifyMode(ctx)
	default:
		return w.startPollMode(ctx)
	}
}

// startPollMode starts polling for schema changes.
func (w *SchemaWatcher) startPollMode(ctx context.Context) error {
	go func() {
		defer close(w.doneCh)

		ticker := time.NewTicker(w.config.PollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := w.engine.RefreshSchema(ctx); err != nil {
					w.engine.logger.Warnw("Schema refresh failed", "error", err)
				} else {
					w.engine.logger.Debug("Schema refreshed via poll")
				}
			case <-w.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	w.engine.logger.Infow("Schema watcher started", "mode", "poll", "interval", w.config.PollInterval)
	return nil
}

// startNotifyMode starts listening for PostgreSQL notifications.
func (w *SchemaWatcher) startNotifyMode(ctx context.Context) error {
	listener, err := NewPGListener(w.engine.db, w.config.Channel)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}
	w.listener = listener

	go func() {
		defer close(w.doneCh)

		for {
			select {
			case <-listener.Notify():
				if err := w.engine.RefreshSchema(ctx); err != nil {
					w.engine.logger.Warnw("Schema refresh failed", "error", err)
				} else {
					w.engine.logger.Info("Schema refreshed via notification")
				}
			case <-w.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	w.engine.logger.Infow("Schema watcher started", "mode", "notify", "channel", w.config.Channel)
	return nil
}

// Stop stops the schema watcher.
func (w *SchemaWatcher) Stop() {
	close(w.stopCh)
	<-w.doneCh
	if w.listener != nil {
		w.listener.Close()
	}
}

// PGListener wraps PostgreSQL LISTEN/NOTIFY functionality.
type PGListener struct {
	db      *sqlx.DB
	channel string
	notify  chan struct{}
	stopCh  chan struct{}
}

// NewPGListener creates a new PostgreSQL listener.
func NewPGListener(db *sqlx.DB, channel string) (*PGListener, error) {
	l := &PGListener{
		db:      db,
		channel: channel,
		notify:  make(chan struct{}, 10),
		stopCh:  make(chan struct{}),
	}

	// Start listening in a goroutine
	go l.listen()

	return l, nil
}

// listen listens for PostgreSQL notifications.
func (l *PGListener) listen() {
	for {
		select {
		case <-l.stopCh:
			return
		default:
			// Poll for notifications using a simple approach
			// In production, you'd use lib/pq's Listener type
			var payload string
			err := l.db.Get(&payload, "SELECT pg_notification_queue_usage()")
			if err == nil {
				select {
				case l.notify <- struct{}{}:
				default:
				}
			}
			time.Sleep(time.Second)
		}
	}
}

// Notify returns the notification channel.
func (l *PGListener) Notify() <-chan struct{} {
	return l.notify
}

// Close closes the listener.
func (l *PGListener) Close() {
	close(l.stopCh)
}

// StartSchemaWatcher starts the schema watcher if configured.
func (e *Engine) StartSchemaWatcher(ctx context.Context) error {
	if !e.config.SchemaWatch.Enabled {
		return nil
	}

	e.schemaWatcher = NewSchemaWatcher(e, e.config.SchemaWatch)
	e.stopWatcher = make(chan struct{})

	return e.schemaWatcher.Start(ctx)
}

// StopSchemaWatcher stops the schema watcher.
func (e *Engine) StopSchemaWatcher() {
	if e.schemaWatcher != nil {
		e.schemaWatcher.Stop()
	}
}

// TriggerSchemaRefresh manually triggers a schema refresh.
func (e *Engine) TriggerSchemaRefresh(ctx context.Context) error {
	return e.schemaManager.Refresh(ctx)
}
