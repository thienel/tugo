package collection

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/thienel/tugo/pkg/apperror"
	"github.com/thienel/tugo/pkg/query"
	"github.com/thienel/tugo/pkg/response"
	"go.uber.org/zap"
)

// Handler handles HTTP requests for collections.
type Handler struct {
	service *Service
	logger  *zap.SugaredLogger
}

// NewHandler creates a new collection handler.
func NewHandler(service *Service, logger *zap.SugaredLogger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// List handles GET /:collection requests.
func (h *Handler) List(c *gin.Context) {
	collectionName := c.Param("collection")

	// Convert query parameters to map
	queryParams := make(map[string][]string)
	for k, v := range c.Request.URL.Query() {
		queryParams[k] = v
	}

	// Parse expand parameter
	expand := query.ParseExpand(queryParams)

	result, err := h.service.List(c.Request.Context(), ListParams{
		CollectionName: collectionName,
		QueryParams:    queryParams,
		Expand:         expand,
	})

	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, response.SuccessList(result.Items, result.Pagination))
}

// Get handles GET /:collection/:id requests.
func (h *Handler) Get(c *gin.Context) {
	collectionName := c.Param("collection")
	id := c.Param("id")

	// Parse expand parameter
	queryParams := make(map[string][]string)
	for k, v := range c.Request.URL.Query() {
		queryParams[k] = v
	}
	expand := query.ParseExpand(queryParams)

	item, err := h.service.Get(c.Request.Context(), collectionName, id, expand)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, response.Success(item))
}

// Create handles POST /:collection requests.
func (h *Handler) Create(c *gin.Context) {
	collectionName := c.Param("collection")

	var data map[string]any
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, response.FromAppError(
			apperror.ErrBadRequest.WithMessage("Invalid JSON body"),
		))
		return
	}

	item, err := h.service.Create(c.Request.Context(), collectionName, data)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, response.Success(item))
}

// Update handles PATCH /:collection/:id requests.
func (h *Handler) Update(c *gin.Context) {
	collectionName := c.Param("collection")
	id := c.Param("id")

	var data map[string]any
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, response.FromAppError(
			apperror.ErrBadRequest.WithMessage("Invalid JSON body"),
		))
		return
	}

	item, err := h.service.Update(c.Request.Context(), collectionName, id, data)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, response.Success(item))
}

// Delete handles DELETE /:collection/:id requests.
func (h *Handler) Delete(c *gin.Context) {
	collectionName := c.Param("collection")
	id := c.Param("id")

	err := h.service.Delete(c.Request.Context(), collectionName, id)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, response.Success(nil))
}

// handleError converts errors to HTTP responses.
func (h *Handler) handleError(c *gin.Context, err error) {
	if appErr, ok := apperror.AsAppError(err); ok {
		c.JSON(appErr.HTTPStatus, response.FromAppError(appErr))
		return
	}

	h.logger.Errorw("Unexpected error", "error", err)
	c.JSON(http.StatusInternalServerError, response.FromAppError(apperror.ErrInternalServer))
}

// RegisterRoutes registers collection routes on a Gin router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/:collection", h.List)
	rg.POST("/:collection", h.Create)
	rg.GET("/:collection/:id", h.Get)
	rg.PATCH("/:collection/:id", h.Update)
	rg.DELETE("/:collection/:id", h.Delete)
}
