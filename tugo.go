package tugo

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/thienel/tlog"
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

	return engine, nil
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
	// Mount collection routes
	e.collHandler.RegisterRoutes(rg)

	e.logger.Infow("TuGo routes mounted", "path", rg.BasePath())
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
