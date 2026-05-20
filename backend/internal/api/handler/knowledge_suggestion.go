package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"caseagent/internal/db/models"
	suggestionservice "caseagent/internal/service/suggestion"

	"github.com/gin-gonic/gin"
)

type updateSuggestionRequest struct {
	Status string `json:"status" binding:"required"`
}

func (h *Handler) ListKnowledgeSuggestions(c *gin.Context) {
	status := c.Query("status")
	if status != "" &&
		status != models.SuggestionStatusPending &&
		status != models.SuggestionStatusAdopted &&
		status != models.SuggestionStatusDismissed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status filter"})
		return
	}

	rows, err := suggestionservice.New(DBFromContext(c)).List(c, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rows)
}

func (h *Handler) UpdateKnowledgeSuggestion(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req updateSuggestionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	row, ok, err := suggestionservice.New(DBFromContext(c)).SetStatus(c, id, req.Status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !ok && row == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be adopted or dismissed"})
		return
	}
	if !ok {
		c.JSON(http.StatusConflict, gin.H{
			"error":  "suggestion is not in pending state",
			"status": row.Status,
		})
		return
	}

	slog.Info("knowledge suggestion status updated",
		"suggestion_id", row.ID, "status", row.Status)

	c.JSON(http.StatusOK, row)
}
