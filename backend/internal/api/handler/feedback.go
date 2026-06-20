package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

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

	row, err := feedbackservice.New(DBFromContext(c)).CreateCaseFeedback(c.Request.Context(), feedbackservice.CreateInput{
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
	rows, err := feedbackservice.New(DBFromContext(c)).ListTaskFeedback(c.Request.Context(), taskID)
	if err != nil {
		writeFeedbackServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, rows)
}

func (h *Handler) GetFeedbackSummary(c *gin.Context) {
	input, ok := parseFeedbackSummaryInput(c)
	if !ok {
		return
	}
	summary, err := feedbackservice.New(DBFromContext(c)).FeedbackSummary(c.Request.Context(), input)
	if err != nil {
		writeFeedbackServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, summary)
}

func (h *Handler) GetQualityOverview(c *gin.Context) {
	input, ok := parseFeedbackSummaryInput(c)
	if !ok {
		return
	}
	overview, err := feedbackservice.New(DBFromContext(c)).QualityOverview(c.Request.Context(), input)
	if err != nil {
		writeFeedbackServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, overview)
}

func parseFeedbackSummaryInput(c *gin.Context) (feedbackservice.SummaryInput, bool) {
	input := feedbackservice.SummaryInput{
		FeedbackType:  strings.TrimSpace(c.Query("feedback_type")),
		PromptID:      strings.TrimSpace(c.Query("prompt_id")),
		PromptVersion: strings.TrimSpace(c.Query("prompt_version")),
	}
	if raw := c.Query("task_id"); raw != "" {
		taskID, err := strconv.Atoi(raw)
		if err != nil || taskID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "task_id must be a positive integer"})
			return input, false
		}
		input.TaskID = taskID
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
