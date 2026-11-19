// Package apperr defines the error type returned by services and turned into
// JSON by httpx. Every error has a stable machine code (UPPER_SNAKE) and a
// Vietnamese message shown to the user.
package apperr

import (
	"errors"
	"net/http"
)

type Error struct {
	Status  int            `json:"-"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// WithDetails returns a copy of e carrying extra machine-readable details.
func (e *Error) WithDetails(d map[string]any) *Error {
	c := *e
	c.Details = d
	return &c
}

func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

func BadRequest(code, msg string) *Error   { return New(http.StatusBadRequest, code, msg) }
func Unauthorized(code, msg string) *Error { return New(http.StatusUnauthorized, code, msg) }
func Forbidden(code, msg string) *Error    { return New(http.StatusForbidden, code, msg) }
func NotFound(code, msg string) *Error     { return New(http.StatusNotFound, code, msg) }
func Conflict(code, msg string) *Error     { return New(http.StatusConflict, code, msg) }

// Common errors shared by all modules.
var (
	ErrUnauthenticated = Unauthorized("UNAUTHENTICATED", "Bạn cần đăng nhập")
	ErrForbidden       = Forbidden("FORBIDDEN", "Bạn không có quyền thực hiện thao tác này")
	ErrNotFound        = NotFound("NOT_FOUND", "Không tìm thấy dữ liệu")
	ErrValidation      = BadRequest("VALIDATION_ERROR", "Dữ liệu không hợp lệ")
	ErrBadOrigin       = Forbidden("BAD_ORIGIN", "Yêu cầu không hợp lệ")
)

// As extracts an *Error from err, or nil.
func As(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}
