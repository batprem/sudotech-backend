package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// parseID extracts a path parameter by name, parses it as int64, and writes a
// 400 response on failure. Returns non-nil error when the caller should abort.
func parseID(c *gin.Context, param string, out *int64) error {
	raw := c.Param(param)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return err
	}
	*out = id
	return nil
}
