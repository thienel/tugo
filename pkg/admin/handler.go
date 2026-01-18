package admin

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/thienel/tugo/pkg/apperror"
	"github.com/thienel/tugo/pkg/response"
	"github.com/thienel/tugo/pkg/schema"
	"github.com/thienel/tugo/pkg/validation"
	"go.uber.org/zap"
)

// Handler handles admin API requests.
type Handler struct {
	schemaManager *schema.Manager
	executor      *SchemaExecutor
	migrationGen  *MigrationGenerator
	logger        *zap.SugaredLogger
	config        HandlerConfig
}

// HandlerConfig configures the admin handler.
type HandlerConfig struct {
	// MigrationsDir is the directory to output migration files.
	// If empty, no migration files are generated.
	MigrationsDir string

	// AutoExecute determines if schema changes are executed immediately.
	// If false, only migration files are generated.
	AutoExecute bool

	// TablePrefix is the prefix for new tables.
	TablePrefix string
}

// DefaultHandlerConfig returns default handler configuration.
func DefaultHandlerConfig() HandlerConfig {
	return HandlerConfig{
		MigrationsDir: "",
		AutoExecute:   true,
		TablePrefix:   "api_",
	}
}

// NewHandler creates a new admin handler.
func NewHandler(schemaManager *schema.Manager, executor *SchemaExecutor, logger *zap.SugaredLogger, config HandlerConfig) *Handler {
	var migrationGen *MigrationGenerator
	if config.MigrationsDir != "" {
		migrationGen = NewMigrationGenerator(config.MigrationsDir)
	}

	return &Handler{
		schemaManager: schemaManager,
		executor:      executor,
		migrationGen:  migrationGen,
		logger:        logger,
		config:        config,
	}
}

// ListCollections handles GET /admin/collections.
func (h *Handler) ListCollections(c *gin.Context) {
	collections := h.schemaManager.ListCollections()

	result := make([]CollectionInfo, 0, len(collections))
	for _, col := range collections {
		result = append(result, toCollectionInfo(col))
	}

	c.JSON(http.StatusOK, response.Success(result))
}

// GetCollection handles GET /admin/collections/:name.
func (h *Handler) GetCollection(c *gin.Context) {
	name := c.Param("name")

	collection, err := h.schemaManager.GetCollection(name)
	if err != nil {
		c.JSON(http.StatusNotFound, response.FromAppError(
			apperror.ErrCollectionNotFound.WithMessage("Collection not found: " + name),
		))
		return
	}

	c.JSON(http.StatusOK, response.Success(toCollectionInfo(collection)))
}

// CreateCollection handles POST /admin/collections.
func (h *Handler) CreateCollection(c *gin.Context) {
	var req CreateCollectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.FromAppError(
			apperror.ErrBadRequest.WithMessage("Invalid request body"),
		))
		return
	}

	// Validate collection name
	if err := validation.ValidateCollectionName(req.Name); err != nil {
		c.JSON(http.StatusBadRequest, response.FromAppError(
			apperror.ErrValidation.WithMessage(err.Error()),
		))
		return
	}

	// Validate field names
	for _, field := range req.Fields {
		if err := validation.ValidateFieldName(field.Name); err != nil {
			c.JSON(http.StatusBadRequest, response.FromAppError(
				apperror.ErrValidation.WithMessage(err.Error()),
			))
			return
		}
	}

	// Ensure table name has prefix
	tableName := req.Name
	if !strings.HasPrefix(tableName, h.config.TablePrefix) {
		tableName = h.config.TablePrefix + tableName
	}
	req.Name = tableName

	// Generate migration if configured
	var migration *Migration
	if h.migrationGen != nil {
		var err error
		migration, err = h.migrationGen.GenerateCreateTable(req)
		if err != nil {
			h.logger.Errorw("Failed to generate migration", "error", err)
			c.JSON(http.StatusInternalServerError, response.FromAppError(
				apperror.ErrInternalServer.WithMessage("Failed to generate migration"),
			))
			return
		}
	}

	// Execute if auto-execute is enabled
	if h.config.AutoExecute && h.executor != nil {
		sql := ""
		if migration != nil {
			sql = migration.UpSQL
		} else if h.migrationGen != nil {
			m, _ := h.migrationGen.GenerateCreateTable(req)
			sql = m.UpSQL
		} else {
			// Generate SQL directly
			m := &MigrationGenerator{}
			mm, _ := m.GenerateCreateTable(req)
			sql = mm.UpSQL
		}

		if err := h.executor.Execute(c.Request.Context(), sql); err != nil {
			h.logger.Errorw("Failed to execute migration", "error", err)
			c.JSON(http.StatusInternalServerError, response.FromAppError(
				apperror.ErrInternalServer.WithMessage("Failed to create table: " + err.Error()),
			))
			return
		}

		// Refresh schema
		if err := h.schemaManager.Refresh(c.Request.Context()); err != nil {
			h.logger.Warnw("Failed to refresh schema after create", "error", err)
		}
	}

	result := gin.H{
		"name":      req.Name,
		"created":   h.config.AutoExecute,
	}
	if migration != nil {
		result["migration"] = gin.H{
			"version":   migration.Version,
			"up_path":   migration.UpPath,
			"down_path": migration.DownPath,
		}
	}

	c.JSON(http.StatusCreated, response.Success(result))
}

// AddField handles POST /admin/collections/:name/fields.
func (h *Handler) AddField(c *gin.Context) {
	collectionName := c.Param("name")

	var req AddFieldRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.FromAppError(
			apperror.ErrBadRequest.WithMessage("Invalid request body"),
		))
		return
	}

	// Validate field name
	if err := validation.ValidateFieldName(req.Field.Name); err != nil {
		c.JSON(http.StatusBadRequest, response.FromAppError(
			apperror.ErrValidation.WithMessage(err.Error()),
		))
		return
	}

	// Check collection exists
	collection, err := h.schemaManager.GetCollection(collectionName)
	if err != nil {
		c.JSON(http.StatusNotFound, response.FromAppError(
			apperror.ErrCollectionNotFound.WithMessage("Collection not found"),
		))
		return
	}

	// Generate migration if configured
	var migration *Migration
	if h.migrationGen != nil {
		migration, err = h.migrationGen.GenerateAddColumn(collection.TableName, req.Field)
		if err != nil {
			h.logger.Errorw("Failed to generate migration", "error", err)
			c.JSON(http.StatusInternalServerError, response.FromAppError(
				apperror.ErrInternalServer.WithMessage("Failed to generate migration"),
			))
			return
		}
	}

	// Execute if auto-execute is enabled
	if h.config.AutoExecute && h.executor != nil {
		sql := ""
		if migration != nil {
			sql = migration.UpSQL
		} else {
			m := &MigrationGenerator{}
			mm, _ := m.GenerateAddColumn(collection.TableName, req.Field)
			sql = mm.UpSQL
		}

		if err := h.executor.Execute(c.Request.Context(), sql); err != nil {
			h.logger.Errorw("Failed to execute migration", "error", err)
			c.JSON(http.StatusInternalServerError, response.FromAppError(
				apperror.ErrInternalServer.WithMessage("Failed to add field: " + err.Error()),
			))
			return
		}

		// Refresh schema
		if err := h.schemaManager.Refresh(c.Request.Context()); err != nil {
			h.logger.Warnw("Failed to refresh schema after add field", "error", err)
		}
	}

	result := gin.H{
		"field":   req.Field.Name,
		"added":   h.config.AutoExecute,
	}
	if migration != nil {
		result["migration"] = gin.H{
			"version":   migration.Version,
			"up_path":   migration.UpPath,
			"down_path": migration.DownPath,
		}
	}

	c.JSON(http.StatusCreated, response.Success(result))
}

// AlterField handles PATCH /admin/collections/:name/fields/:field.
func (h *Handler) AlterField(c *gin.Context) {
	collectionName := c.Param("name")
	fieldName := c.Param("field")

	var req AlterFieldRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.FromAppError(
			apperror.ErrBadRequest.WithMessage("Invalid request body"),
		))
		return
	}

	// Check collection exists
	collection, err := h.schemaManager.GetCollection(collectionName)
	if err != nil {
		c.JSON(http.StatusNotFound, response.FromAppError(
			apperror.ErrCollectionNotFound.WithMessage("Collection not found"),
		))
		return
	}

	// Generate migration if configured
	var migration *Migration
	if h.migrationGen != nil {
		migration, err = h.migrationGen.GenerateAlterColumn(collection.TableName, fieldName, req)
		if err != nil {
			h.logger.Errorw("Failed to generate migration", "error", err)
			c.JSON(http.StatusInternalServerError, response.FromAppError(
				apperror.ErrInternalServer.WithMessage("Failed to generate migration"),
			))
			return
		}
	}

	// Execute if auto-execute is enabled
	if h.config.AutoExecute && h.executor != nil {
		sql := ""
		if migration != nil {
			sql = migration.UpSQL
		} else {
			m := &MigrationGenerator{}
			mm, _ := m.GenerateAlterColumn(collection.TableName, fieldName, req)
			sql = mm.UpSQL
		}

		if err := h.executor.Execute(c.Request.Context(), sql); err != nil {
			h.logger.Errorw("Failed to execute migration", "error", err)
			c.JSON(http.StatusInternalServerError, response.FromAppError(
				apperror.ErrInternalServer.WithMessage("Failed to alter field: " + err.Error()),
			))
			return
		}

		// Refresh schema
		if err := h.schemaManager.Refresh(c.Request.Context()); err != nil {
			h.logger.Warnw("Failed to refresh schema after alter field", "error", err)
		}
	}

	result := gin.H{
		"field":   fieldName,
		"altered": h.config.AutoExecute,
	}
	if migration != nil {
		result["migration"] = gin.H{
			"version":   migration.Version,
			"up_path":   migration.UpPath,
			"down_path": migration.DownPath,
		}
	}

	c.JSON(http.StatusOK, response.Success(result))
}

// DeleteField handles DELETE /admin/collections/:name/fields/:field.
func (h *Handler) DeleteField(c *gin.Context) {
	collectionName := c.Param("name")
	fieldName := c.Param("field")

	// Check collection exists
	collection, err := h.schemaManager.GetCollection(collectionName)
	if err != nil {
		c.JSON(http.StatusNotFound, response.FromAppError(
			apperror.ErrCollectionNotFound.WithMessage("Collection not found"),
		))
		return
	}

	// Generate migration if configured
	var migration *Migration
	if h.migrationGen != nil {
		migration, err = h.migrationGen.GenerateDropColumn(collection.TableName, fieldName)
		if err != nil {
			h.logger.Errorw("Failed to generate migration", "error", err)
			c.JSON(http.StatusInternalServerError, response.FromAppError(
				apperror.ErrInternalServer.WithMessage("Failed to generate migration"),
			))
			return
		}
	}

	// Execute if auto-execute is enabled
	if h.config.AutoExecute && h.executor != nil {
		sql := ""
		if migration != nil {
			sql = migration.UpSQL
		} else {
			m := &MigrationGenerator{}
			mm, _ := m.GenerateDropColumn(collection.TableName, fieldName)
			sql = mm.UpSQL
		}

		if err := h.executor.Execute(c.Request.Context(), sql); err != nil {
			h.logger.Errorw("Failed to execute migration", "error", err)
			c.JSON(http.StatusInternalServerError, response.FromAppError(
				apperror.ErrInternalServer.WithMessage("Failed to delete field: " + err.Error()),
			))
			return
		}

		// Refresh schema
		if err := h.schemaManager.Refresh(c.Request.Context()); err != nil {
			h.logger.Warnw("Failed to refresh schema after delete field", "error", err)
		}
	}

	result := gin.H{
		"field":   fieldName,
		"deleted": h.config.AutoExecute,
	}
	if migration != nil {
		result["migration"] = gin.H{
			"version":   migration.Version,
			"up_path":   migration.UpPath,
			"down_path": migration.DownPath,
		}
	}

	c.JSON(http.StatusOK, response.Success(result))
}

// DeleteCollection handles DELETE /admin/collections/:name.
func (h *Handler) DeleteCollection(c *gin.Context) {
	collectionName := c.Param("name")

	// Check collection exists
	collection, err := h.schemaManager.GetCollection(collectionName)
	if err != nil {
		c.JSON(http.StatusNotFound, response.FromAppError(
			apperror.ErrCollectionNotFound.WithMessage("Collection not found"),
		))
		return
	}

	// Generate migration if configured
	var migration *Migration
	if h.migrationGen != nil {
		migration, err = h.migrationGen.GenerateDropTable(collection.TableName)
		if err != nil {
			h.logger.Errorw("Failed to generate migration", "error", err)
			c.JSON(http.StatusInternalServerError, response.FromAppError(
				apperror.ErrInternalServer.WithMessage("Failed to generate migration"),
			))
			return
		}
	}

	// Execute if auto-execute is enabled
	if h.config.AutoExecute && h.executor != nil {
		sql := ""
		if migration != nil {
			sql = migration.UpSQL
		} else {
			m := &MigrationGenerator{}
			mm, _ := m.GenerateDropTable(collection.TableName)
			sql = mm.UpSQL
		}

		if err := h.executor.Execute(c.Request.Context(), sql); err != nil {
			h.logger.Errorw("Failed to execute migration", "error", err)
			c.JSON(http.StatusInternalServerError, response.FromAppError(
				apperror.ErrInternalServer.WithMessage("Failed to delete collection: " + err.Error()),
			))
			return
		}

		// Refresh schema
		if err := h.schemaManager.Refresh(c.Request.Context()); err != nil {
			h.logger.Warnw("Failed to refresh schema after delete collection", "error", err)
		}
	}

	result := gin.H{
		"name":    collectionName,
		"deleted": h.config.AutoExecute,
	}
	if migration != nil {
		result["migration"] = gin.H{
			"version":   migration.Version,
			"up_path":   migration.UpPath,
			"down_path": migration.DownPath,
		}
	}

	c.JSON(http.StatusOK, response.Success(result))
}

// SyncSchema handles POST /admin/sync-schema.
func (h *Handler) SyncSchema(c *gin.Context) {
	if err := h.schemaManager.Refresh(c.Request.Context()); err != nil {
		h.logger.Errorw("Failed to sync schema", "error", err)
		c.JSON(http.StatusInternalServerError, response.FromAppError(
			apperror.ErrInternalServer.WithMessage("Failed to sync schema"),
		))
		return
	}

	collections := h.schemaManager.ListCollections()
	c.JSON(http.StatusOK, response.Success(gin.H{
		"synced":      true,
		"collections": len(collections),
	}))
}

// RegisterRoutes registers admin routes on a Gin router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/collections", h.ListCollections)
	rg.POST("/collections", h.CreateCollection)
	rg.GET("/collections/:name", h.GetCollection)
	rg.DELETE("/collections/:name", h.DeleteCollection)
	rg.POST("/collections/:name/fields", h.AddField)
	rg.PATCH("/collections/:name/fields/:field", h.AlterField)
	rg.DELETE("/collections/:name/fields/:field", h.DeleteField)
	rg.POST("/sync-schema", h.SyncSchema)
}

// toCollectionInfo converts a schema.Collection to CollectionInfo.
func toCollectionInfo(col *schema.Collection) CollectionInfo {
	fields := make([]FieldInfo, 0, len(col.Fields))
	for _, f := range col.Fields {
		var defaultVal *string
		if f.DefaultValue != nil {
			defaultVal = f.DefaultValue
		}
		fields = append(fields, FieldInfo{
			Name:         f.Name,
			Type:         f.DataType,
			PostgresType: f.PostgresType,
			Required:     !f.IsNullable,
			Unique:       f.IsUnique,
			Primary:      f.IsPrimaryKey,
			Default:      defaultVal,
			MaxLength:    f.MaxLength,
		})
	}

	return CollectionInfo{
		Name:       col.Name,
		TableName:  col.TableName,
		Enabled:    col.Enabled,
		Fields:     fields,
		PrimaryKey: col.PrimaryKey,
	}
}

// SchemaExecutor executes schema modification SQL.
type SchemaExecutor struct {
	db DBExecutor
}

// DBExecutor is the interface for executing SQL statements.
type DBExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) error
}

// sqlxDBWrapper wraps sqlx.DB to implement DBExecutor.
type sqlxDBWrapper struct {
	db *sqlx.DB
}

// ExecContext implements DBExecutor.
func (w *sqlxDBWrapper) ExecContext(ctx context.Context, query string, args ...any) error {
	_, err := w.db.ExecContext(ctx, query, args...)
	return err
}

// NewSchemaExecutor creates a new schema executor from sqlx.DB.
func NewSchemaExecutor(db *sqlx.DB) *SchemaExecutor {
	return &SchemaExecutor{db: &sqlxDBWrapper{db: db}}
}

// Execute executes SQL statements.
func (e *SchemaExecutor) Execute(ctx context.Context, sql string) error {
	return e.db.ExecContext(ctx, sql)
}
