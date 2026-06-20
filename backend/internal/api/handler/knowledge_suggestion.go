package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"caseagent/internal/db/models"
	suggestionservice "caseagent/internal/service/suggestion"

	"github.com/gin-gonic/gin"
)

type updateSuggestionRequest struct {
	Status              string `json:"status" binding:"required"`
	ResolvedKnowledgeID *int   `json:"resolved_knowledge_id"`
}

type createSuggestionRequest struct {
	CandidateType   string `json:"candidate_type" binding:"required"`
	CandidateName   string `json:"candidate_name" binding:"required"`
	SourceCaseID    int    `json:"source_case_id" binding:"required"`
	SourceTaskID    int    `json:"source_task_id" binding:"required"`
	SourceCaseTitle string `json:"source_case_title"`
	Note            string `json:"note"`
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

	rows, err := suggestionservice.New(DBFromContext(c)).List(c.Request.Context(), status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rows)
}

func (h *Handler) CreateKnowledgeSuggestion(c *gin.Context) {
	var req createSuggestionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	input := manualSuggestionInput(req)
	if err := validateManualSuggestionRequest(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": strings.TrimPrefix(err.Error(), suggestionservice.ErrInvalidManualSuggestion.Error()+": ")})
		return
	}

	row, err := suggestionservice.New(DBFromContext(c)).CreateManual(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, suggestionservice.ErrInvalidManualSuggestion) {
			c.JSON(http.StatusBadRequest, gin.H{"error": strings.TrimPrefix(err.Error(), suggestionservice.ErrInvalidManualSuggestion.Error()+": ")})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slog.Info("manual knowledge suggestion created",
		"suggestion_id", row.ID,
		"task_id", row.SourceTaskID,
		"case_id", row.SourceCaseID,
	)

	c.JSON(http.StatusCreated, row)
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

	row, ok, err := suggestionservice.New(DBFromContext(c)).SetStatus(c.Request.Context(), id, req.Status, req.ResolvedKnowledgeID)
	if err != nil {
		if errors.Is(err, suggestionservice.ErrInvalidManualSuggestion) {
			c.JSON(http.StatusBadRequest, gin.H{"error": strings.TrimPrefix(err.Error(), suggestionservice.ErrInvalidManualSuggestion.Error()+": ")})
			return
		}
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

func (h *Handler) DraftKnowledgeSuggestion(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	result, found, err := suggestionservice.New(DBFromContext(c)).Draft(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, suggestionservice.ErrInvalidManualSuggestion) {
			c.JSON(http.StatusBadRequest, gin.H{"error": strings.TrimPrefix(err.Error(), suggestionservice.ErrInvalidManualSuggestion.Error()+": ")})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "suggestion not found"})
		return
	}

	c.JSON(http.StatusOK, result)
}

func manualSuggestionInput(req createSuggestionRequest) suggestionservice.ManualSuggestionInput {
	return suggestionservice.ManualSuggestionInput{
		CandidateType:   strings.TrimSpace(req.CandidateType),
		CandidateName:   strings.TrimSpace(req.CandidateName),
		SourceTaskID:    req.SourceTaskID,
		SourceCaseID:    req.SourceCaseID,
		SourceCaseTitle: strings.TrimSpace(req.SourceCaseTitle),
		Note:            strings.TrimSpace(req.Note),
	}
}

func validateManualSuggestionRequest(req createSuggestionRequest) error {
	return suggestionservice.ValidateManualSuggestionInput(manualSuggestionInput(req))
}
