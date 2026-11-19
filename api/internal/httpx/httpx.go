// Package httpx holds HTTP helpers shared by every module: JSON responses,
// error rendering, request binding and pagination.
package httpx

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/ndhuy-sg/first-claude-app/api/internal/apperr"
)

// ErrorBody is the JSON shape of every error response:
// {"error": {"code": "ORDER_PAID", "message": "Đơn đã thanh toán", "details": {...}}}
type ErrorBody struct {
	Error *apperr.Error `json:"error"`
}

// Fail writes err as JSON. Unknown errors become 500 and are logged.
// pgx.ErrNoRows becomes 404 NOT_FOUND.
func Fail(c *gin.Context, err error) {
	if e := apperr.As(err); e != nil {
		c.AbortWithStatusJSON(e.Status, ErrorBody{Error: e})
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		c.AbortWithStatusJSON(http.StatusNotFound, ErrorBody{Error: apperr.ErrNotFound})
		return
	}
	slog.ErrorContext(c.Request.Context(), "internal error", "err", err, "path", c.FullPath())
	c.AbortWithStatusJSON(http.StatusInternalServerError, ErrorBody{Error: apperr.New(500, "INTERNAL", "Có lỗi xảy ra, vui lòng thử lại")})
}

// OK writes v as JSON with status 200.
func OK(c *gin.Context, v any) { c.JSON(http.StatusOK, v) }

// Created writes v as JSON with status 201.
func Created(c *gin.Context, v any) { c.JSON(http.StatusCreated, v) }

// Bind decodes the JSON body into dst. On failure it returns a 400 VALIDATION_ERROR.
func Bind(c *gin.Context, dst any) error {
	if err := c.ShouldBindJSON(dst); err != nil {
		return apperr.ErrValidation.WithDetails(map[string]any{"reason": err.Error()})
	}
	return nil
}

// PathID parses a positive int64 path parameter. Invalid ids become 404.
func PathID(c *gin.Context, name string) (int64, error) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.ErrNotFound
	}
	return id, nil
}

// Page holds pagination query params (?page=1&pageSize=50), 1-based, pageSize capped at 200.
type Page struct {
	Page     int
	PageSize int
}

func (p Page) Limit() int32  { return int32(p.PageSize) }
func (p Page) Offset() int32 { return int32((p.Page - 1) * p.PageSize) }

func GetPage(c *gin.Context) Page {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("pageSize", "50"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 50
	}
	if size > 200 {
		size = 200
	}
	return Page{Page: page, PageSize: size}
}

// List is the JSON shape of paginated lists.
type List[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}
