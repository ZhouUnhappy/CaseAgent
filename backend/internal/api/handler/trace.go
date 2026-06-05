package handler

import (
	"net/http"

	"caseagent/internal/db/models"

	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

type taskTraceView struct {
	WorkflowRuns  []models.WorkflowRun  `json:"workflow_runs"`
	Steps         []models.WorkflowStep `json:"steps"`
	AgentRuns     []models.AgentRun     `json:"agent_runs"`
	ModelCalls    []models.ModelCall    `json:"model_calls"`
	RetrievalRuns []models.RetrievalRun `json:"retrieval_runs"`
	Artifacts     []models.Artifact     `json:"artifacts"`
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
	return nil
}

func newTaskTraceView(runs []models.WorkflowRun) taskTraceView {
	if runs == nil {
		runs = []models.WorkflowRun{}
	}
	return taskTraceView{
		WorkflowRuns:  runs,
		Steps:         []models.WorkflowStep{},
		AgentRuns:     []models.AgentRun{},
		ModelCalls:    []models.ModelCall{},
		RetrievalRuns: []models.RetrievalRun{},
		Artifacts:     []models.Artifact{},
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
