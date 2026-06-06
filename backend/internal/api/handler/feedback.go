package handler

import (
	"errors"
	"net/http"

	feedbackservice "caseagent/internal/service/feedback"

	"github.com/gin-gonic/gin"
)

type createCaseFeedbackRequest struct {
	CaseIndex    int            `json:"case_index"`
	FeedbackType string         `json:"feedback_type" binding:"required"`
	Note         string         `json:"note"`
	Metadata     map[string]any `json:"metadata"`
}

func (h *Handler) CreateCaseFeedback(c *gin.Context) {
	taskID, ok := parseTaskID(c)
	if !ok {
		return
	}
	caseID, ok := parsePositiveIDParam(c, "case_id", "case ID")
	if !ok {
		return
	}

	var req createCaseFeedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	row, err := feedbackservice.New(DBFromContext(c)).CreateCaseFeedback(c, feedbackservice.CreateInput{
		TaskID:       taskID,
		TestCaseID:   caseID,
		CaseIndex:    req.CaseIndex,
		FeedbackType: req.FeedbackType,
		Note:         req.Note,
		Metadata:     req.Metadata,
	})
	if err != nil {
		writeFeedbackServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, row)
}

func (h *Handler) ListTaskFeedback(c *gin.Context) {
	taskID, ok := parseTaskID(c)
	if !ok {
		return
	}
	rows, err := feedbackservice.New(DBFromContext(c)).ListTaskFeedback(c, taskID)
	if err != nil {
		writeFeedbackServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, rows)
}

func writeFeedbackServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, feedbackservice.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, feedbackservice.ErrTestCaseNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "Test case not found"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
