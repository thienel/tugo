package response

import (
	"github.com/thienel/tugo/pkg/apperror"
)

// Response is the standard API response structure.
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorBody  `json:"error,omitempty"`
}

// ErrorBody contains error details.
type ErrorBody struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// ListData wraps list responses with pagination.
type ListData struct {
	Items      interface{} `json:"items"`
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
func Success(data interface{}) Response {
	return Response{
		Success: true,
		Data:    data,
	}
}

// SuccessList creates a successful list response with pagination.
func SuccessList(items interface{}, pagination *Pagination) Response {
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
func ErrorWithDetails(code, message string, details interface{}) Response {
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
