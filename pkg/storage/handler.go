package storage

import (
	"mime"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/thienel/tugo/pkg/apperror"
	"github.com/thienel/tugo/pkg/response"
	"go.uber.org/zap"
)

// Handler handles HTTP requests for file operations.
type Handler struct {
	manager *Manager
	logger  *zap.SugaredLogger
	config  HandlerConfig
}

// HandlerConfig configures the file handler.
type HandlerConfig struct {
	// MaxUploadSize is the maximum upload size in bytes.
	MaxUploadSize int64

	// AllowedTypes is a list of allowed MIME types. Empty means all types.
	AllowedTypes []string

	// DefaultProvider is the default storage provider to use.
	DefaultProvider string
}

// DefaultHandlerConfig returns default handler configuration.
func DefaultHandlerConfig() HandlerConfig {
	return HandlerConfig{
		MaxUploadSize:   50 * 1024 * 1024, // 50MB
		AllowedTypes:    nil,              // Allow all
		DefaultProvider: "",               // Use manager's default
	}
}

// NewHandler creates a new file handler.
func NewHandler(manager *Manager, logger *zap.SugaredLogger, config HandlerConfig) *Handler {
	return &Handler{
		manager: manager,
		logger:  logger,
		config:  config,
	}
}

// Upload handles POST /files/upload requests.
func (h *Handler) Upload(c *gin.Context) {
	// Parse multipart form
	if err := c.Request.ParseMultipartForm(h.config.MaxUploadSize); err != nil {
		c.JSON(http.StatusBadRequest, response.FromAppError(
			apperror.ErrBadRequest.WithMessage("Failed to parse form"),
		))
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, response.FromAppError(
			apperror.ErrBadRequest.WithMessage("No file provided"),
		))
		return
	}
	defer file.Close()

	// Check file size
	if header.Size > h.config.MaxUploadSize {
		c.JSON(http.StatusBadRequest, response.FromAppError(
			apperror.ErrBadRequest.WithMessage("File too large"),
		))
		return
	}

	// Detect content type
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		ext := filepath.Ext(header.Filename)
		contentType = mime.TypeByExtension(ext)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
	}

	// Check allowed types
	if len(h.config.AllowedTypes) > 0 {
		allowed := false
		for _, t := range h.config.AllowedTypes {
			if t == contentType {
				allowed = true
				break
			}
		}
		if !allowed {
			c.JSON(http.StatusBadRequest, response.FromAppError(
				apperror.ErrBadRequest.WithMessage("File type not allowed"),
			))
			return
		}
	}

	// Get optional directory from form
	directory := c.PostForm("directory")

	// Get optional provider from form
	provider := c.PostForm("provider")
	if provider == "" {
		provider = h.config.DefaultProvider
	}

	// Upload file
	record, err := h.manager.Upload(c.Request.Context(), provider, file, header.Filename, &UploadOptions{
		ContentType: contentType,
		MaxSize:     h.config.MaxUploadSize,
		Directory:   directory,
	})
	if err != nil {
		h.logger.Errorw("Failed to upload file", "error", err)
		c.JSON(http.StatusInternalServerError, response.FromAppError(
			apperror.ErrInternalServer.WithMessage("Failed to upload file"),
		))
		return
	}

	c.JSON(http.StatusCreated, response.Success(gin.H{
		"id":           record.ID,
		"filename":     record.Filename,
		"url":          record.URL,
		"size":         record.Size,
		"content_type": record.ContentType,
	}))
}

// Download handles GET /files/:id requests.
func (h *Handler) Download(c *gin.Context) {
	fileID := c.Param("id")

	reader, record, err := h.manager.Download(c.Request.Context(), fileID)
	if err != nil {
		h.logger.Warnw("Failed to download file", "id", fileID, "error", err)
		c.JSON(http.StatusNotFound, response.FromAppError(
			apperror.ErrNotFound.WithMessage("File not found"),
		))
		return
	}
	defer reader.Close()

	// Set headers
	c.Header("Content-Type", record.ContentType)
	c.Header("Content-Disposition", "attachment; filename=\""+record.Filename+"\"")
	c.Header("Content-Length", strconv.FormatInt(record.Size, 10))

	// Stream file
	c.DataFromReader(http.StatusOK, record.Size, record.ContentType, reader, nil)
}

// Get handles GET /files/:id/info requests.
func (h *Handler) Get(c *gin.Context) {
	fileID := c.Param("id")

	record, err := h.manager.GetFileRecord(c.Request.Context(), fileID)
	if err != nil {
		c.JSON(http.StatusNotFound, response.FromAppError(
			apperror.ErrNotFound.WithMessage("File not found"),
		))
		return
	}

	c.JSON(http.StatusOK, response.Success(record))
}

// Delete handles DELETE /files/:id requests.
func (h *Handler) Delete(c *gin.Context) {
	fileID := c.Param("id")

	err := h.manager.Delete(c.Request.Context(), fileID)
	if err != nil {
		h.logger.Warnw("Failed to delete file", "id", fileID, "error", err)
		c.JSON(http.StatusNotFound, response.FromAppError(
			apperror.ErrNotFound.WithMessage("File not found"),
		))
		return
	}

	c.JSON(http.StatusOK, response.Success(nil))
}

// List handles GET /files requests.
func (h *Handler) List(c *gin.Context) {
	// Parse pagination
	page := 1
	limit := 20

	if p := c.Query("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}

	offset := (page - 1) * limit

	records, total, err := h.manager.ListFiles(c.Request.Context(), limit, offset)
	if err != nil {
		h.logger.Errorw("Failed to list files", "error", err)
		c.JSON(http.StatusInternalServerError, response.FromAppError(
			apperror.ErrInternalServer.WithMessage("Failed to list files"),
		))
		return
	}

	c.JSON(http.StatusOK, response.SuccessList(records, response.NewPagination(page, limit, total)))
}

// RegisterRoutes registers file routes on a Gin router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/upload", h.Upload)
	rg.GET("", h.List)
	rg.GET("/:id", h.Download)
	rg.GET("/:id/info", h.Get)
	rg.DELETE("/:id", h.Delete)
}
