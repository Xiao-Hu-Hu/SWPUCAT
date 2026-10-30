package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Response struct {
	Code     int         `json:"code"`
	Message  string      `json:"message"`
	Data     interface{} `json:"data,omitempty"`
	Warnings []string    `json:"warnings,omitempty"`
}

func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}

func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Response{
		Code:    0,
		Message: "created",
		Data:    data,
	})
}

// CreatedWithWarnings mirrors Created but attaches non-blocking advisory
// warnings (e.g. a description that is too short to index well). The request
// still succeeded; warnings are informational only.
func CreatedWithWarnings(c *gin.Context, data interface{}, warnings []string) {
	c.JSON(http.StatusCreated, Response{
		Code:     0,
		Message:  "created",
		Data:     data,
		Warnings: warnings,
	})
}

func Error(c *gin.Context, httpStatus int, code int, message string) {
	c.JSON(httpStatus, Response{
		Code:    code,
		Message: message,
	})
}

func BadRequest(c *gin.Context, message string) {
	Error(c, http.StatusBadRequest, 400, message)
}

func Unauthorized(c *gin.Context, message string) {
	Error(c, http.StatusUnauthorized, 401, message)
}

func Forbidden(c *gin.Context, message string) {
	Error(c, http.StatusForbidden, 403, message)
}

func NotFound(c *gin.Context, message string) {
	Error(c, http.StatusNotFound, 404, message)
}

func Conflict(c *gin.Context, message string) {
	Error(c, http.StatusConflict, 409, message)
}

func RequestTooLarge(c *gin.Context, message string) {
	Error(c, http.StatusRequestEntityTooLarge, 413, message)
}

func InternalError(c *gin.Context, message string) {
	Error(c, http.StatusInternalServerError, 500, message)
}
