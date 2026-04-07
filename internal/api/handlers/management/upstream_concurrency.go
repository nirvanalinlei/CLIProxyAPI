package management

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) GetUpstreamConcurrency(c *gin.Context) {
	if h == nil || h.authManager == nil {
		c.JSON(http.StatusOK, gin.H{"upstream-concurrency": []any{}})
		return
	}
	items := h.authManager.LocalConcurrencySnapshots()
	if len(items) == 0 {
		c.JSON(http.StatusOK, gin.H{"upstream-concurrency": []any{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"upstream-concurrency": items})
}
