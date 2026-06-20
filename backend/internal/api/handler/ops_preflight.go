package handler

import (
	"net/http"

	"caseagent/internal/config"
	"caseagent/internal/service/opscheck"

	"github.com/gin-gonic/gin"
)

func (h *Handler) GetOpsPreflight(c *gin.Context) {
	report, err := opscheck.New(DBFromContext(c), config.Get()).Get(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, report)
}
