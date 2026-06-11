package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"caseagent/internal/config"
	retentionservice "caseagent/internal/service/retention"

	"github.com/gin-gonic/gin"
)

type retentionCleanupRequest struct {
	RetentionDays int    `json:"retention_days"`
	Reason        string `json:"reason"`
}

func (h *Handler) GetRetentionCleanupPlan(c *gin.Context) {
	retentionDays, ok := parseRetentionDays(c.Query("retention_days"), configuredTraceRetentionDays(), c)
	if !ok {
		return
	}
	report, err := retentionservice.New(DBFromContext(c)).Cleanup(c.Request.Context(), retentionservice.Input{
		RetentionDays: retentionDays,
	})
	if err != nil {
		writeRetentionError(c, err)
		return
	}
	c.JSON(http.StatusOK, report)
}

func (h *Handler) RunRetentionCleanup(c *gin.Context) {
	var req retentionCleanupRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "reason is required"})
		return
	}
	retentionDays := req.RetentionDays
	if retentionDays == 0 {
		retentionDays = configuredTraceRetentionDays()
	}
	if retentionDays <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "retention_days must be > 0"})
		return
	}
	operator := parseTrustedOperator(c)
	report, err := retentionservice.New(DBFromContext(c)).Cleanup(c.Request.Context(), retentionservice.Input{
		RetentionDays: retentionDays,
		Execute:       true,
		OperatorID:    operator.ID,
		OperatorName:  operator.Name,
		Reason:        req.Reason,
	})
	if err != nil {
		writeRetentionError(c, err)
		return
	}
	c.JSON(http.StatusOK, report)
}

func configuredTraceRetentionDays() int {
	cfg := config.Get()
	if cfg == nil || cfg.Retention.TraceRetentionDays <= 0 {
		return 30
	}
	return cfg.Retention.TraceRetentionDays
}

func parseRetentionDays(raw string, fallback int, c *gin.Context) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "retention_days must be > 0"})
		return 0, false
	}
	return value, true
}

func writeRetentionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, retentionservice.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, retentionservice.ErrMissingTenant):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
