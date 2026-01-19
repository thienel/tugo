package response

import (
	"github.com/gin-gonic/gin"
	"github.com/thienel/tugo/pkg/apperror"
)

// Response is the standard API response structure.
type Response struct {
	Success bool       `json:"success"`
	Data    any        `json:"data,omitempty"`
	Error   *ErrorBody `json:"error,omitempty"`
}

// ErrorBody contains error details.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// ListData wraps list responses with pagination.
type ListData struct {
	Items      any         `json:"items"`
	Pagination *Pagination `json:"pagination,omitempty"`
}

// Pagination contains pagination metadata.
type Pagination struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// Success creates a successful response.
func Success(data any) Response {
	return Response{
		Success: true,
		Data:    data,
	}
}

// SuccessList creates a successful list response with pagination.
func SuccessList(items any, pagination *Pagination) Response {
	return Response{
		Success: true,
		Data: ListData{
			Items:      items,
			Pagination: pagination,
		},
	}
}

// Error creates an error response.
func Error(code, message string) Response {
	return Response{
		Success: false,
		Error: &ErrorBody{
			Code:    code,
			Message: message,
		},
	}
}

// ErrorWithDetails creates an error response with details.
func ErrorWithDetails(code, message string, details any) Response {
	return Response{
		Success: false,
		Error: &ErrorBody{
			Code:    code,
			Message: message,
			Details: details,
		},
	}
}

// FromAppError creates an error response from an AppError.
func FromAppError(err *apperror.AppError) Response {
	return Response{
		Success: false,
		Error: &ErrorBody{
			Code:    err.Code,
			Message: err.Message,
			Details: err.Details,
		},
	}
}

// FromValidationErrors creates an error response from validation errors.
func FromValidationErrors(errs *apperror.ValidationErrors) Response {
	return Response{
		Success: false,
		Error: &ErrorBody{
			Code:    apperror.ErrValidation.Code,
			Message: apperror.ErrValidation.Message,
			Details: errs.Errors,
		},
	}
}

// NewPagination creates pagination metadata.
func NewPagination(page, limit, total int) *Pagination {
	totalPages := total / limit
	if total%limit > 0 {
		totalPages++
	}
	return &Pagination{
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}
}

// Gin helper functions for common HTTP responses

// JSON sends a JSON response with status code.
func JSON(c *gin.Context, status int, resp Response) {
	c.JSON(status, resp)
}

// OK sends a 200 OK response with data.
func OK(c *gin.Context, data any) {
	c.JSON(200, Success(data))
}

// Created sends a 201 Created response with data.
func Created(c *gin.Context, data any) {
	c.JSON(201, Success(data))
}

// NoContent sends a 204 No Content response.
func NoContent(c *gin.Context) {
	c.Status(204)
}

// BadRequest sends a 400 Bad Request response.
func BadRequest(c *gin.Context, message string) {
	c.JSON(400, Error(apperror.CodeBadRequest, message))
}

// Unauthorized sends a 401 Unauthorized response.
func Unauthorized(c *gin.Context, message string) {
	c.JSON(401, Error(apperror.CodeUnauthorized, message))
}

// Forbidden sends a 403 Forbidden response.
func Forbidden(c *gin.Context, message string) {
	c.JSON(403, Error(apperror.CodeForbidden, message))
}

// NotFound sends a 404 Not Found response.
func NotFound(c *gin.Context, message string) {
	c.JSON(404, Error(apperror.CodeNotFound, message))
}

// Conflict sends a 409 Conflict response.
func Conflict(c *gin.Context, message string) {
	c.JSON(409, Error(apperror.CodeConflict, message))
}

// ValidationError sends a 422 Unprocessable Entity response.
func ValidationError(c *gin.Context, message string, details any) {
	c.JSON(422, ErrorWithDetails(apperror.CodeValidation, message, details))
}

// InternalError sends a 500 Internal Server Error response.
func InternalError(c *gin.Context, message string) {
	c.JSON(500, Error(apperror.CodeInternalServer, message))
}

// HandleAppError sends appropriate response based on AppError.
func HandleAppError(c *gin.Context, err *apperror.AppError) {
	c.JSON(err.HTTPStatus, FromAppError(err))
}
