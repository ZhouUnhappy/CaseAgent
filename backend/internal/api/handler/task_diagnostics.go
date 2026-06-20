package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"caseagent/internal/db/models"
	feedbackservice "caseagent/internal/service/feedback"

	"github.com/gin-gonic/gin"
)

const maxDiagnosticMetadataStringChars = 512

type taskDiagnosticsPackage struct {
	GeneratedAt time.Time                 `json:"generated_at"`
	Task        models.CaseGenerationTask `json:"task"`
	TestCases   []diagnosticTestCase      `json:"test_cases"`
	Trace       taskTraceView             `json:"trace"`
	Summary     taskDiagnosticsSummary    `json:"summary"`
	Redaction   taskDiagnosticsRedaction  `json:"redaction"`
}

type diagnosticTestCase struct {
	ID                   int              `json:"id"`
	TenantID             int              `json:"tenant_id"`
	TaskID               int              `json:"task_id"`
	Section              string           `json:"section"`
	Cases                []map[string]any `json:"cases"`
	SourceContextSummary map[string]any   `json:"source_context_summary,omitempty"`
	Status               string           `json:"status"`
	CreatedAt            time.Time        `json:"created_at"`
	UpdatedAt            time.Time        `json:"updated_at"`
}

type taskDiagnosticsSummary struct {
	SectionCount       int                              `json:"section_count"`
	CaseCount          int                              `json:"case_count"`
	WorkflowRuns       int                              `json:"workflow_runs"`
	AgentRuns          int                              `json:"agent_runs"`
	ModelCalls         int                              `json:"model_calls"`
	RetrievalRuns      int                              `json:"retrieval_runs"`
	Artifacts          int                              `json:"artifacts"`
	FeedbackCount      int                              `json:"feedback_count"`
	AgentRunsByStatus  map[string]int                   `json:"agent_runs_by_status"`
	ModelCallsByStatus map[string]int                   `json:"model_calls_by_status"`
	RetrievalsByStatus map[string]int                   `json:"retrieval_runs_by_status"`
	FeedbackByType     map[string]int                   `json:"feedback_by_type"`
	Errors             []diagnosticErrorSummary         `json:"errors,omitempty"`
	SourceContexts     []diagnosticSourceContextSummary `json:"source_contexts"`
	LastError          string                           `json:"last_error,omitempty"`
}

type diagnosticErrorSummary struct {
	Source    string `json:"source"`
	ID        int    `json:"id"`
	Status    string `json:"status,omitempty"`
	LastError string `json:"last_error"`
}

type diagnosticSourceContextSummary struct {
	TestCaseID int            `json:"test_case_id"`
	Section    string         `json:"section"`
	CaseCount  int            `json:"case_count"`
	Summary    map[string]any `json:"summary"`
}

type taskDiagnosticsRedaction struct {
	Enabled                bool     `json:"enabled"`
	MaxMetadataStringChars int      `json:"max_metadata_string_chars"`
	Rules                  []string `json:"rules"`
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
		Scan(c.Request.Context()); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	var testCases []models.TestCase
	if err := DBFromContext(c).NewSelect().
		Model(&testCases).
		Where("task_id = ?", taskID).
		Order("id ASC").
		Scan(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var runs []models.WorkflowRun
	if err := DBFromContext(c).NewSelect().
		Model(&runs).
		Where("resource_type = ?", "task").
		Where("resource_id = ?", taskID).
		Order("created_at ASC", "id ASC").
		Scan(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	view := newTaskTraceView(runs)
	if err := scanTraceRows(c, taskID, workflowRunIDs(runs), &view); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	diagnosticCases := buildDiagnosticTestCases(testCases)
	diagnosticTrace := redactDiagnosticsTrace(view)
	c.JSON(http.StatusOK, taskDiagnosticsPackage{
		GeneratedAt: time.Now(),
		Task:        task,
		TestCases:   diagnosticCases,
		Trace:       diagnosticTrace,
		Summary:     buildTaskDiagnosticsSummary(testCases, diagnosticTrace),
		Redaction:   diagnosticRedactionPolicy(),
	})
}

func buildTaskDiagnosticsSummary(testCases []models.TestCase, trace taskTraceView) taskDiagnosticsSummary {
	caseCount := 0
	for _, section := range testCases {
		caseCount += len(section.Cases)
	}
	return taskDiagnosticsSummary{
		SectionCount:       len(testCases),
		CaseCount:          caseCount,
		WorkflowRuns:       len(trace.WorkflowRuns),
		AgentRuns:          len(trace.AgentRuns),
		ModelCalls:         len(trace.ModelCalls),
		RetrievalRuns:      len(trace.RetrievalRuns),
		Artifacts:          len(trace.Artifacts),
		FeedbackCount:      feedbackSummaryTotal(trace.FeedbackSummary),
		AgentRunsByStatus:  agentRunStatusCounts(trace.AgentRuns),
		ModelCallsByStatus: modelCallStatusCounts(trace.ModelCalls),
		RetrievalsByStatus: retrievalRunStatusCounts(trace.RetrievalRuns),
		FeedbackByType:     trace.FeedbackSummary,
		Errors:             diagnosticErrors(trace),
		SourceContexts:     diagnosticSourceContexts(testCases),
		LastError:          latestTraceError(trace),
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

func buildDiagnosticTestCases(testCases []models.TestCase) []diagnosticTestCase {
	rows := make([]diagnosticTestCase, 0, len(testCases))
	for _, section := range testCases {
		rows = append(rows, diagnosticTestCase{
			ID:                   section.ID,
			TenantID:             section.TenantID,
			TaskID:               section.TaskID,
			Section:              section.Section,
			Cases:                redactCases(section.Cases),
			SourceContextSummary: diagnosticSourceContextSummaryMap(section.SourceContext),
			Status:               section.Status,
			CreatedAt:            section.CreatedAt,
			UpdatedAt:            section.UpdatedAt,
		})
	}
	return rows
}

func redactDiagnosticsTrace(trace taskTraceView) taskTraceView {
	out := trace
	out.WorkflowRuns = append([]models.WorkflowRun(nil), trace.WorkflowRuns...)
	for idx := range out.WorkflowRuns {
		out.WorkflowRuns[idx].Metadata = redactMap(out.WorkflowRuns[idx].Metadata)
	}
	out.Steps = append([]models.WorkflowStep(nil), trace.Steps...)
	for idx := range out.Steps {
		out.Steps[idx].Metadata = redactMap(out.Steps[idx].Metadata)
	}
	out.AgentRuns = append([]models.AgentRun(nil), trace.AgentRuns...)
	for idx := range out.AgentRuns {
		out.AgentRuns[idx].Metadata = redactMap(out.AgentRuns[idx].Metadata)
		out.AgentRuns[idx].InputSummary = redactLongString(out.AgentRuns[idx].InputSummary)
		out.AgentRuns[idx].OutputSummary = redactLongString(out.AgentRuns[idx].OutputSummary)
	}
	out.ModelCalls = append([]models.ModelCall(nil), trace.ModelCalls...)
	for idx := range out.ModelCalls {
		out.ModelCalls[idx].Metadata = redactMap(out.ModelCalls[idx].Metadata)
	}
	out.RetrievalRuns = append([]models.RetrievalRun(nil), trace.RetrievalRuns...)
	for idx := range out.RetrievalRuns {
		out.RetrievalRuns[idx].Metadata = redactMap(out.RetrievalRuns[idx].Metadata)
	}
	out.Artifacts = append([]models.Artifact(nil), trace.Artifacts...)
	for idx := range out.Artifacts {
		out.Artifacts[idx].Content = redactArtifactContent(out.Artifacts[idx].Content)
		out.Artifacts[idx].Payload = redactMap(out.Artifacts[idx].Payload)
	}
	out.CaseProvenance = append([]caseProvenanceView(nil), trace.CaseProvenance...)
	for idx := range out.CaseProvenance {
		out.CaseProvenance[idx].SourceContext = diagnosticSourceContextSummaryMap(out.CaseProvenance[idx].SourceContext)
		out.CaseProvenance[idx].DocumentQueries = queryCount(out.CaseProvenance[idx].DocumentQueries)
		out.CaseProvenance[idx].KnowledgeQueries = queryCount(out.CaseProvenance[idx].KnowledgeQueries)
		out.CaseProvenance[idx].DocumentHits = summarizeHitList(out.CaseProvenance[idx].DocumentHits, "document_id", "name", "rank", "best_score")
		out.CaseProvenance[idx].KnowledgeHits = summarizeHitList(out.CaseProvenance[idx].KnowledgeHits, "id", "name", "type", "rank", "score")
		out.CaseProvenance[idx].AgentRuns = redactAgentProvenance(out.CaseProvenance[idx].AgentRuns)
		out.CaseProvenance[idx].ModelCalls = redactModelProvenance(out.CaseProvenance[idx].ModelCalls)
		out.CaseProvenance[idx].Feedback = redactFeedbackRows(out.CaseProvenance[idx].Feedback)
	}
	out.Feedback = redactFeedbackRows(trace.Feedback)
	return out
}

func diagnosticRedactionPolicy() taskDiagnosticsRedaction {
	return taskDiagnosticsRedaction{
		Enabled:                true,
		MaxMetadataStringChars: maxDiagnosticMetadataStringChars,
		Rules: []string{
			"artifact.content is replaced with a length marker",
			"source_context is summarized without raw query text or hit content",
			"metadata keys containing secret, token, password, authorization, credential, api_key, access_key, secret_key, prompt, content, body, raw, or text are redacted",
			"long metadata strings are replaced with length markers",
		},
	}
}

func diagnosticSourceContexts(testCases []models.TestCase) []diagnosticSourceContextSummary {
	rows := make([]diagnosticSourceContextSummary, 0, len(testCases))
	for _, section := range testCases {
		rows = append(rows, diagnosticSourceContextSummary{
			TestCaseID: section.ID,
			Section:    section.Section,
			CaseCount:  len(section.Cases),
			Summary:    diagnosticSourceContextSummaryMap(section.SourceContext),
		})
	}
	return rows
}

func diagnosticSourceContextSummaryMap(source map[string]any) map[string]any {
	if source == nil {
		return map[string]any{}
	}
	summary := feedbackservice.SummarizeSourceContext(source)
	delete(summary, "document_queries")
	delete(summary, "knowledge_queries")
	summary["document_query_count"] = queryCount(source["document_queries"])
	summary["knowledge_query_count"] = queryCount(source["knowledge_queries"])
	for _, key := range []string{"generation_profile_id", "generation_profile_version", "generation_profile_name"} {
		if value, ok := source[key]; ok {
			summary[key] = redactValue(key, value)
		}
	}
	return redactMap(summary)
}

func diagnosticErrors(trace taskTraceView) []diagnosticErrorSummary {
	rows := []diagnosticErrorSummary{}
	for _, run := range trace.WorkflowRuns {
		if run.LastError != "" {
			rows = append(rows, diagnosticErrorSummary{Source: "workflow_run", ID: run.ID, Status: run.Status, LastError: run.LastError})
		}
	}
	for _, step := range trace.Steps {
		if step.LastError != "" {
			rows = append(rows, diagnosticErrorSummary{Source: "workflow_step", ID: step.ID, Status: step.Status, LastError: step.LastError})
		}
	}
	for _, agent := range trace.AgentRuns {
		if agent.LastError != "" {
			rows = append(rows, diagnosticErrorSummary{Source: "agent_run", ID: agent.ID, Status: agent.Status, LastError: agent.LastError})
		}
	}
	for _, call := range trace.ModelCalls {
		if call.LastError != "" {
			rows = append(rows, diagnosticErrorSummary{Source: "model_call", ID: call.ID, Status: call.Status, LastError: call.LastError})
		}
	}
	for _, run := range trace.RetrievalRuns {
		if run.LastError != "" {
			rows = append(rows, diagnosticErrorSummary{Source: "retrieval_run", ID: run.ID, Status: run.Status, LastError: run.LastError})
		}
	}
	return rows
}

func agentRunStatusCounts(rows []models.AgentRun) map[string]int {
	counts := map[string]int{}
	for _, row := range rows {
		counts[row.Status]++
	}
	return counts
}

func modelCallStatusCounts(rows []models.ModelCall) map[string]int {
	counts := map[string]int{}
	for _, row := range rows {
		counts[row.Status]++
	}
	return counts
}

func retrievalRunStatusCounts(rows []models.RetrievalRun) map[string]int {
	counts := map[string]int{}
	for _, row := range rows {
		counts[row.Status]++
	}
	return counts
}

func redactCases(cases []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(cases))
	for _, item := range cases {
		out = append(out, redactMap(item))
	}
	return out
}

func redactAgentProvenance(rows []agentRunProvenance) []agentRunProvenance {
	out := append([]agentRunProvenance(nil), rows...)
	for idx := range out {
		out[idx].Metadata = redactMap(out[idx].Metadata)
	}
	return out
}

func redactModelProvenance(rows []modelCallProvenance) []modelCallProvenance {
	out := append([]modelCallProvenance(nil), rows...)
	for idx := range out {
		out[idx].Metadata = redactMap(out[idx].Metadata)
	}
	return out
}

func redactFeedbackRows(rows []models.TestCaseFeedback) []models.TestCaseFeedback {
	out := append([]models.TestCaseFeedback(nil), rows...)
	for idx := range out {
		out[idx].SourceContextSummary = redactMap(out[idx].SourceContextSummary)
		out[idx].Metadata = redactMap(out[idx].Metadata)
		out[idx].Note = redactLongString(out[idx].Note)
	}
	return out
}

func redactMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = redactValue(key, value)
	}
	return out
}

func redactValue(key string, value any) any {
	if shouldRedactKey(key) {
		return redactedMarker(key, value)
	}
	switch typed := value.(type) {
	case map[string]any:
		return redactMap(typed)
	case []map[string]any:
		rows := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			rows = append(rows, redactMap(item))
		}
		return rows
	case []any:
		rows := make([]any, 0, len(typed))
		for _, item := range typed {
			rows = append(rows, redactValue("", item))
		}
		return rows
	case string:
		return redactLongString(typed)
	default:
		return value
	}
}

func shouldRedactKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	for _, token := range []string{
		"secret",
		"token",
		"password",
		"authorization",
		"credential",
		"api_key",
		"access_key",
		"secret_key",
		"prompt",
		"content",
		"body",
		"raw",
		"text",
		"source_context",
		"cases",
	} {
		if strings.Contains(normalized, token) {
			return true
		}
	}
	return false
}

func redactLongString(value string) string {
	if utf8.RuneCountInString(value) <= maxDiagnosticMetadataStringChars {
		return value
	}
	return fmt.Sprintf("[truncated; chars=%d]", utf8.RuneCountInString(value))
}

func redactArtifactContent(value string) string {
	if value == "" {
		return ""
	}
	return fmt.Sprintf("[redacted artifact content; chars=%d]", utf8.RuneCountInString(value))
}

func redactedMarker(key string, value any) string {
	if text, ok := value.(string); ok {
		return fmt.Sprintf("[redacted %s; chars=%d]", key, utf8.RuneCountInString(text))
	}
	return fmt.Sprintf("[redacted %s]", key)
}

func summarizeHitList(value any, keys ...string) []map[string]any {
	rows := mapsFromAny(value)
	maxRows := 5
	if len(rows) < maxRows {
		maxRows = len(rows)
	}
	out := make([]map[string]any, 0, maxRows)
	for idx := 0; idx < maxRows; idx++ {
		item := make(map[string]any, len(keys))
		for _, key := range keys {
			if value, ok := rows[idx][key]; ok {
				item[key] = redactValue(key, value)
			}
		}
		out = append(out, item)
	}
	return out
}

func queryCount(value any) int {
	switch typed := value.(type) {
	case []string:
		return len(typed)
	case []map[string]any:
		return len(typed)
	case []any:
		return len(typed)
	case string:
		if strings.TrimSpace(typed) == "" {
			return 0
		}
		return 1
	default:
		return 0
	}
}
