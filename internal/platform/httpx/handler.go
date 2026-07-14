package httpx

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"

	"whatsapp-ai-poc/internal/platform/apperror"
)

type Handler func(*gin.Context) error

func Adapt(handler Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := handler(c); err != nil {
			WriteError(c, err)
		}
	}
}

func WriteError(c *gin.Context, err error) {
	if c.Writer.Written() {
		c.Abort()
		return
	}

	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		slog.Default().Error("request failed",
			"request_id", RequestIDFrom(c),
			"error_type", fmt.Sprintf("%T", err),
		)
		appErr = apperror.Internal(err)
	}
	payload := gin.H{
		"code":      appErr.Code,
		"message":   appErr.Message,
		"requestId": RequestIDFrom(c),
	}
	if appErr.Details != nil {
		payload["details"] = appErr.Details
	}
	c.AbortWithStatusJSON(appErr.Status, gin.H{"error": payload})
}

func NoRoute(c *gin.Context) error {
	return apperror.NotFound()
}
