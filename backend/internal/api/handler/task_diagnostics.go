package handler

import (
	"net/http"
	"time"

	"caseagent/internal/db/models"

	"github.com/gin-gonic/gin"
)

type taskDiagnosticsPackage struct {
	GeneratedAt time.Time                 `json:"generated_at"`
	Task        models.CaseGenerationTask `json:"task"`
	TestCases   []models.TestCase         `json:"test_cases"`
	Trace       taskTraceView             `json:"trace"`
	Summary     taskDiagnosticsSummary    `json:"summary"`
}

type taskDiagnosticsSummary struct {
	SectionCount  int    `json:"section_count"`
	CaseCount     int    `json:"case_count"`
	WorkflowRuns  int    `json:"workflow_runs"`
	AgentRuns     int    `json:"agent_runs"`
	ModelCalls    int    `json:"model_calls"`
	RetrievalRuns int    `json:"retrieval_runs"`
	Artifacts     int    `json:"artifacts"`
	FeedbackCount int    `json:"feedback_count"`
	LastError     string `json:"last_error,omitempty"`
}

func (h *Handler) GetTaskDiagnostics(c *gin.Context) {
	taskID, ok := parseTaskID(c)
	if !ok {
		return
	}

	var task models.CaseGenerationTask
	if err := DBFromContext(c).NewSelect().
		Model(&task).
		Where("id = ?", taskID).
		Scan(c); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	var testCases []models.TestCase
	if err := DBFromContext(c).NewSelect().
		Model(&testCases).
		Where("task_id = ?", taskID).
		Order("id ASC").
		Scan(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var runs []models.WorkflowRun
	if err := DBFromContext(c).NewSelect().
		Model(&runs).
		Where("resource_type = ?", "task").
		Where("resource_id = ?", taskID).
		Order("created_at ASC", "id ASC").
		Scan(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	view := newTaskTraceView(runs)
	if err := scanTraceRows(c, taskID, workflowRunIDs(runs), &view); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, taskDiagnosticsPackage{
		GeneratedAt: time.Now(),
		Task:        task,
		TestCases:   testCases,
		Trace:       view,
		Summary:     buildTaskDiagnosticsSummary(testCases, view),
	})
}

func buildTaskDiagnosticsSummary(testCases []models.TestCase, trace taskTraceView) taskDiagnosticsSummary {
	caseCount := 0
	for _, section := range testCases {
		caseCount += len(section.Cases)
	}
	return taskDiagnosticsSummary{
		SectionCount:  len(testCases),
		CaseCount:     caseCount,
		WorkflowRuns:  len(trace.WorkflowRuns),
		AgentRuns:     len(trace.AgentRuns),
		ModelCalls:    len(trace.ModelCalls),
		RetrievalRuns: len(trace.RetrievalRuns),
		Artifacts:     len(trace.Artifacts),
		FeedbackCount: feedbackSummaryTotal(trace.FeedbackSummary),
		LastError:     latestTraceError(trace),
	}
}

func feedbackSummaryTotal(counts map[string]int) int {
	total := 0
	for _, count := range counts {
		total += count
	}
	return total
}

func latestTraceError(trace taskTraceView) string {
	for idx := len(trace.WorkflowRuns) - 1; idx >= 0; idx-- {
		if trace.WorkflowRuns[idx].LastError != "" {
			return trace.WorkflowRuns[idx].LastError
		}
	}
	for idx := len(trace.AgentRuns) - 1; idx >= 0; idx-- {
		if trace.AgentRuns[idx].LastError != "" {
			return trace.AgentRuns[idx].LastError
		}
	}
	for idx := len(trace.ModelCalls) - 1; idx >= 0; idx-- {
		if trace.ModelCalls[idx].LastError != "" {
			return trace.ModelCalls[idx].LastError
		}
	}
	return ""
}
