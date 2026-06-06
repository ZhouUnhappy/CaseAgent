package handler

import (
	"net/http"

	"caseagent/internal/db/models"

	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

type taskTraceView struct {
	WorkflowRuns    []models.WorkflowRun      `json:"workflow_runs"`
	Steps           []models.WorkflowStep     `json:"steps"`
	AgentRuns       []models.AgentRun         `json:"agent_runs"`
	ModelCalls      []models.ModelCall        `json:"model_calls"`
	RetrievalRuns   []models.RetrievalRun     `json:"retrieval_runs"`
	Artifacts       []models.Artifact         `json:"artifacts"`
	CaseProvenance  []caseProvenanceView      `json:"case_provenance"`
	Feedback        []models.TestCaseFeedback `json:"feedback"`
	FeedbackSummary map[string]int            `json:"feedback_summary"`
}

type caseProvenanceView struct {
	TestCaseID       int                       `json:"test_case_id"`
	Section          string                    `json:"section"`
	CaseIndex        int                       `json:"case_index"`
	CaseTitle        string                    `json:"case_title"`
	SourceContext    map[string]any            `json:"source_context,omitempty"`
	DocumentQueries  any                       `json:"document_queries,omitempty"`
	KnowledgeQueries any                       `json:"knowledge_queries,omitempty"`
	DocumentHits     any                       `json:"document_hits,omitempty"`
	KnowledgeHits    any                       `json:"knowledge_hits,omitempty"`
	AgentRuns        []agentRunProvenance      `json:"agent_runs"`
	ModelCalls       []modelCallProvenance     `json:"model_calls"`
	Feedback         []models.TestCaseFeedback `json:"feedback,omitempty"`
	FeedbackCounts   map[string]int            `json:"feedback_counts,omitempty"`
}

type agentRunProvenance struct {
	ID       int            `json:"id"`
	Agent    string         `json:"agent"`
	Stage    string         `json:"stage"`
	Status   string         `json:"status"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type modelCallProvenance struct {
	ID            int            `json:"id"`
	AgentRunID    int            `json:"agent_run_id,omitempty"`
	Agent         string         `json:"agent,omitempty"`
	Attempt       string         `json:"attempt,omitempty"`
	Provider      string         `json:"provider,omitempty"`
	Model         string         `json:"model,omitempty"`
	ProviderRole  string         `json:"provider_role,omitempty"`
	Status        string         `json:"status"`
	PromptID      string         `json:"prompt_id,omitempty"`
	PromptVersion string         `json:"prompt_version,omitempty"`
	PromptChars   int            `json:"prompt_chars"`
	ResponseChars int            `json:"response_chars"`
	LastError     string         `json:"last_error,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

func (h *Handler) GetTaskTrace(c *gin.Context) {
	taskID, ok := parseTaskID(c)
	if !ok {
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
	runIDs := workflowRunIDs(runs)
	view := newTaskTraceView(runs)
	if err := scanTraceRows(c, taskID, runIDs, &view); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, view)
}

func scanTraceRows(c *gin.Context, taskID int, runIDs []int, view *taskTraceView) error {
	db := DBFromContext(c)
	if len(runIDs) > 0 {
		if err := db.NewSelect().
			Model(&view.Steps).
			Where("workflow_run_id IN (?)", bun.In(runIDs)).
			Order("created_at ASC", "id ASC").
			Scan(c); err != nil {
			return err
		}
	}

	agentQuery := db.NewSelect().
		Model(&view.AgentRuns).
		Order("created_at ASC", "id ASC")
	if len(runIDs) > 0 {
		agentQuery.Where("(workflow_run_id IN (?) OR task_id = ?)", bun.In(runIDs), taskID)
	} else {
		agentQuery.Where("task_id = ?", taskID)
	}
	if err := agentQuery.Scan(c); err != nil {
		return err
	}

	modelCallQuery := db.NewSelect().
		Model(&view.ModelCalls).
		Order("created_at ASC", "id ASC")
	agentIDs := agentRunIDs(view.AgentRuns)
	switch {
	case len(runIDs) > 0 && len(agentIDs) > 0:
		modelCallQuery.Where("(workflow_run_id IN (?) OR agent_run_id IN (?))", bun.In(runIDs), bun.In(agentIDs))
	case len(runIDs) > 0:
		modelCallQuery.Where("workflow_run_id IN (?)", bun.In(runIDs))
	case len(agentIDs) > 0:
		modelCallQuery.Where("agent_run_id IN (?)", bun.In(agentIDs))
	default:
		modelCallQuery = nil
	}
	if modelCallQuery != nil {
		if err := modelCallQuery.Scan(c); err != nil {
			return err
		}
	}

	retrievalQuery := db.NewSelect().
		Model(&view.RetrievalRuns).
		Order("created_at ASC", "id ASC")
	if len(runIDs) > 0 {
		retrievalQuery.Where("(workflow_run_id IN (?) OR task_id = ?)", bun.In(runIDs), taskID)
	} else {
		retrievalQuery.Where("task_id = ?", taskID)
	}
	if err := retrievalQuery.Scan(c); err != nil {
		return err
	}

	artifactQuery := db.NewSelect().
		Model(&view.Artifacts).
		Order("created_at ASC", "id ASC")
	if len(runIDs) > 0 {
		artifactQuery.Where("(workflow_run_id IN (?) OR (resource_type = ? AND resource_id = ?))", bun.In(runIDs), "task", taskID)
	} else {
		artifactQuery.Where("resource_type = ? AND resource_id = ?", "task", taskID)
	}
	if err := artifactQuery.Scan(c); err != nil {
		return err
	}
	var testCases []models.TestCase
	if err := db.NewSelect().
		Model(&testCases).
		Where("task_id = ?", taskID).
		Order("id ASC").
		Scan(c); err != nil {
		return err
	}
	if err := db.NewSelect().
		Model(&view.Feedback).
		Where("task_id = ?", taskID).
		Order("created_at DESC", "id DESC").
		Scan(c); err != nil {
		return err
	}
	view.FeedbackSummary = feedbackCounts(view.Feedback)
	view.CaseProvenance = buildCaseProvenanceViews(testCases, view.AgentRuns, view.ModelCalls, view.Feedback)
	return nil
}

func newTaskTraceView(runs []models.WorkflowRun) taskTraceView {
	if runs == nil {
		runs = []models.WorkflowRun{}
	}
	return taskTraceView{
		WorkflowRuns:    runs,
		Steps:           []models.WorkflowStep{},
		AgentRuns:       []models.AgentRun{},
		ModelCalls:      []models.ModelCall{},
		RetrievalRuns:   []models.RetrievalRun{},
		Artifacts:       []models.Artifact{},
		CaseProvenance:  []caseProvenanceView{},
		Feedback:        []models.TestCaseFeedback{},
		FeedbackSummary: map[string]int{},
	}
}

func workflowRunIDs(runs []models.WorkflowRun) []int {
	ids := make([]int, 0, len(runs))
	for _, run := range runs {
		ids = append(ids, run.ID)
	}
	return ids
}

func agentRunIDs(runs []models.AgentRun) []int {
	ids := make([]int, 0, len(runs))
	for _, run := range runs {
		ids = append(ids, run.ID)
	}
	return ids
}

func buildCaseProvenanceViews(testCases []models.TestCase, agentRuns []models.AgentRun, modelCalls []models.ModelCall, feedbackRows []models.TestCaseFeedback) []caseProvenanceView {
	agentByID := make(map[int]models.AgentRun, len(agentRuns))
	for _, agent := range agentRuns {
		agentByID[agent.ID] = agent
	}
	modelByID := make(map[int]models.ModelCall, len(modelCalls))
	for _, call := range modelCalls {
		modelByID[call.ID] = call
	}
	feedbackByCase := groupFeedbackByCase(feedbackRows)

	rows := make([]caseProvenanceView, 0)
	for _, section := range testCases {
		source := section.SourceContext
		modelIDs := idsFromSourceContext(source, "model_calls")
		agentIDs := idsFromSourceContext(source, "agent_runs")
		if len(modelIDs) == 0 {
			for _, call := range modelCalls {
				modelIDs = append(modelIDs, call.ID)
			}
		}
		for _, id := range modelIDs {
			if call, ok := modelByID[id]; ok && call.AgentRunID != nil {
				agentIDs = appendUniqueInt(agentIDs, *call.AgentRunID)
			}
		}
		for idx, item := range section.Cases {
			caseFeedback := feedbackByCase[caseFeedbackKey{TestCaseID: section.ID, CaseIndex: idx}]
			rows = append(rows, caseProvenanceView{
				TestCaseID:       section.ID,
				Section:          section.Section,
				CaseIndex:        idx,
				CaseTitle:        stringFromAny(item["title"]),
				SourceContext:    source,
				DocumentQueries:  sourceValue(source, "document_queries"),
				KnowledgeQueries: sourceValue(source, "knowledge_queries"),
				DocumentHits:     sourceValue(source, "document_hits"),
				KnowledgeHits:    sourceValue(source, "knowledge_hits"),
				AgentRuns:        agentProvenance(agentIDs, agentByID),
				ModelCalls:       modelProvenance(modelIDs, modelByID),
				Feedback:         caseFeedback,
				FeedbackCounts:   feedbackCounts(caseFeedback),
			})
		}
	}
	return rows
}

type caseFeedbackKey struct {
	TestCaseID int
	CaseIndex  int
}

func groupFeedbackByCase(rows []models.TestCaseFeedback) map[caseFeedbackKey][]models.TestCaseFeedback {
	grouped := make(map[caseFeedbackKey][]models.TestCaseFeedback)
	for _, row := range rows {
		key := caseFeedbackKey{TestCaseID: row.TestCaseID, CaseIndex: row.CaseIndex}
		grouped[key] = append(grouped[key], row)
	}
	return grouped
}

func feedbackCounts(rows []models.TestCaseFeedback) map[string]int {
	counts := map[string]int{}
	for _, row := range rows {
		if row.FeedbackType == "" {
			continue
		}
		counts[row.FeedbackType]++
	}
	return counts
}

func agentProvenance(ids []int, agentByID map[int]models.AgentRun) []agentRunProvenance {
	rows := make([]agentRunProvenance, 0, len(ids))
	for _, id := range ids {
		agent, ok := agentByID[id]
		if !ok {
			continue
		}
		rows = append(rows, agentRunProvenance{
			ID:       agent.ID,
			Agent:    agent.AgentName,
			Stage:    agent.Stage,
			Status:   agent.Status,
			Metadata: agent.Metadata,
		})
	}
	return rows
}

func modelProvenance(ids []int, modelByID map[int]models.ModelCall) []modelCallProvenance {
	rows := make([]modelCallProvenance, 0, len(ids))
	for _, id := range ids {
		call, ok := modelByID[id]
		if !ok {
			continue
		}
		row := modelCallProvenance{
			ID:            call.ID,
			Provider:      call.Provider,
			Model:         call.Model,
			Status:        call.Status,
			PromptChars:   call.PromptChars,
			ResponseChars: call.ResponseChars,
			LastError:     call.LastError,
			Metadata:      call.Metadata,
		}
		if call.AgentRunID != nil {
			row.AgentRunID = *call.AgentRunID
		}
		if call.Metadata != nil {
			row.Agent = stringFromAny(call.Metadata["agent"])
			row.Attempt = stringFromAny(call.Metadata["attempt"])
			row.ProviderRole = stringFromAny(call.Metadata["provider_role"])
			row.PromptID = stringFromAny(call.Metadata["prompt_id"])
			row.PromptVersion = stringFromAny(call.Metadata["prompt_version"])
		}
		rows = append(rows, row)
	}
	return rows
}

func idsFromSourceContext(source map[string]any, key string) []int {
	items := mapsFromAny(sourceValue(source, key))
	ids := make([]int, 0, len(items))
	for _, item := range items {
		ids = appendUniqueInt(ids, intFromAny(item["id"]))
	}
	return ids
}

func sourceValue(source map[string]any, key string) any {
	if source == nil {
		return nil
	}
	return source[key]
}

func mapsFromAny(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		rows := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok {
				rows = append(rows, mapped)
			}
		}
		return rows
	default:
		return nil
	}
}

func appendUniqueInt(ids []int, id int) []int {
	if id <= 0 {
		return ids
	}
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func stringFromAny(value any) string {
	text, _ := value.(string)
	return text
}
