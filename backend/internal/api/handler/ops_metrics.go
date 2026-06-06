package handler

import (
	"net/http"
	"strconv"
	"time"

	"caseagent/internal/service/opsmetrics"

	"github.com/gin-gonic/gin"
)

func (h *Handler) GetOpsMetrics(c *gin.Context) {
	input, ok := parseOpsMetricsInput(c)
	if !ok {
		return
	}
	view, err := opsmetrics.New(DBFromContext(c)).Get(c, input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, view)
}

func parseOpsMetricsInput(c *gin.Context) (opsmetrics.Input, bool) {
	input := opsmetrics.Input{
		Provider:     c.Query("provider"),
		Model:        c.Query("model"),
		WorkflowType: c.Query("workflow_type"),
	}
	if raw := c.Query("task_id"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "task_id must be a positive integer"})
			return input, false
		}
		input.TaskID = id
	}
	from, ok := parseOptionalOpsTime(c, "from")
	if !ok {
		return input, false
	}
	to, ok := parseOptionalOpsTime(c, "to")
	if !ok {
		return input, false
	}
	input.From = from
	input.To = to
	return input, true
}

func parseOptionalOpsTime(c *gin.Context, key string) (*time.Time, bool) {
	raw := c.Query(key)
	if raw == "" {
		return nil, true
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		parsed, err := time.Parse(layout, raw)
		if err == nil {
			return &parsed, true
		}
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": key + " must be RFC3339 or YYYY-MM-DD"})
	return nil, false
}
