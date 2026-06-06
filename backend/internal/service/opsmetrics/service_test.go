package opsmetrics

import (
	"testing"
	"time"

	"caseagent/internal/db/models"
)

func TestAggregateRowsEmpty(t *testing.T) {
	view := aggregateRows(Input{}, rows{})
	if view.Summary.ModelCalls != 0 || view.Summary.WorkflowRuns != 0 || view.Summary.BackgroundJobs != 0 {
		t.Fatalf("unexpected non-empty summary: %#v", view.Summary)
	}
	if len(view.ByModel) != 0 || len(view.ByWorkflow) != 0 || len(view.FailureStages) != 0 || len(view.JobStatuses) != 0 {
		t.Fatalf("expected empty metric slices, got %#v", view)
	}
}

func TestAggregateRowsCostAndStability(t *testing.T) {
	start := time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC)
	finishFast := start.Add(2 * time.Second)
	finishSlow := start.Add(8 * time.Second)
	taskID := 77
	run1 := 1001
	run2 := 1002
	agent1 := 2001
	agent2 := 2002

	view := aggregateRows(Input{}, rows{
		WorkflowRuns: []models.WorkflowRun{
			{ID: run1, WorkflowType: models.JobTypeGenerate, ResourceType: "task", ResourceID: taskID, Status: models.WorkflowStatusSucceeded, StartedAt: &start, FinishedAt: &finishFast, CreatedAt: start, UpdatedAt: finishFast},
			{ID: run2, WorkflowType: models.JobTypeGenerate, ResourceType: "task", ResourceID: taskID, Status: models.WorkflowStatusFailed, StartedAt: &start, FinishedAt: &finishSlow, CreatedAt: start, UpdatedAt: finishSlow},
		},
		AgentRuns: []models.AgentRun{
			{ID: agent1, WorkflowRunID: &run1, TaskID: &taskID, AgentName: "functional", Stage: "generate", Status: models.WorkflowStatusSucceeded, CreatedAt: start, UpdatedAt: finishFast},
			{ID: agent2, WorkflowRunID: &run2, TaskID: &taskID, AgentName: "functional", Stage: "parse", Status: models.WorkflowStatusFailed, LastError: "bad json", CreatedAt: start, UpdatedAt: finishSlow},
		},
		ModelCalls: []models.ModelCall{
			{ID: 3001, WorkflowRunID: &run1, AgentRunID: &agent1, Provider: "openai", Model: "gpt-5", Status: models.WorkflowStatusSucceeded, PromptChars: 100, ResponseChars: 300, Metadata: map[string]any{"cost": map[string]any{"accounted_tokens": 100}, "elapsed_ms": 1100, "provider_role": "primary"}, CreatedAt: start, UpdatedAt: finishFast},
			{ID: 3002, WorkflowRunID: &run2, AgentRunID: &agent2, Provider: "openai", Model: "gpt-5", Status: models.WorkflowStatusFailed, PromptChars: 50, ResponseChars: 0, LastError: "rate limit exceeded", Metadata: map[string]any{"cost": map[string]any{"estimated_total_tokens": 13}, "elapsed_ms": 500, "provider_role": "fallback"}, CreatedAt: start, UpdatedAt: finishSlow},
			{ID: 3003, WorkflowRunID: &run2, AgentRunID: &agent2, Provider: "openai", Model: "gpt-5-mini", Status: models.WorkflowStatusFailed, PromptChars: 80, ResponseChars: 0, LastError: "circuit open", Metadata: map[string]any{"guardrail_event": "circuit_open"}, CreatedAt: start, UpdatedAt: finishSlow},
			{ID: 3004, WorkflowRunID: &run2, AgentRunID: &agent2, Provider: "openai", Model: "gpt-5-mini", Status: models.WorkflowStatusFailed, PromptChars: 40, ResponseChars: 0, LastError: "budget exceeded", Metadata: map[string]any{"guardrail_event": "budget_exceeded"}, CreatedAt: start, UpdatedAt: finishSlow},
		},
		BackgroundJobs: []models.BackgroundJob{
			{ID: 4001, TaskID: &taskID, WorkflowRunID: &run1, JobType: models.JobTypeGenerate, Status: models.JobStatusSucceeded, CreatedAt: start, UpdatedAt: finishFast},
			{ID: 4002, TaskID: &taskID, WorkflowRunID: &run2, JobType: models.JobTypeGenerate, Status: models.JobStatusFailed, CreatedAt: start, UpdatedAt: finishSlow},
			{ID: 4003, TaskID: &taskID, JobType: models.JobTypeGenerate, Status: models.JobStatusPending, RetryCount: 1, CreatedAt: start, UpdatedAt: finishSlow},
		},
	})

	if view.Summary.ModelCalls != 4 || view.Summary.ModelSucceeded != 1 || view.Summary.ModelFailed != 3 {
		t.Fatalf("unexpected model call summary: %#v", view.Summary)
	}
	if view.Summary.AccountedTokens != 143 {
		t.Fatalf("accounted tokens = %d, want 143", view.Summary.AccountedTokens)
	}
	if view.Summary.Fallbacks != 1 || view.Summary.RateLimits != 1 || view.Summary.CircuitOpen != 1 || view.Summary.BudgetExceeded != 1 {
		t.Fatalf("unexpected stability signals: %#v", view.Summary)
	}
	if view.Summary.WorkflowRuns != 2 || view.Summary.WorkflowSucceeded != 1 || view.Summary.WorkflowFailed != 1 || view.Summary.WorkflowSuccessRate != 0.5 {
		t.Fatalf("unexpected workflow summary: %#v", view.Summary)
	}
	if view.Summary.AverageWorkflowMs != 5000 {
		t.Fatalf("average workflow ms = %d, want 5000", view.Summary.AverageWorkflowMs)
	}
	if len(view.FailureStages) != 1 || view.FailureStages[0].Stage != "parse" || view.FailureStages[0].Failures != 1 {
		t.Fatalf("unexpected failure stages: %#v", view.FailureStages)
	}
	if len(view.ByModel) != 2 || view.ByModel[0].Provider != "openai" || view.ByModel[0].AccountedTokens != 113 {
		t.Fatalf("unexpected model metrics: %#v", view.ByModel)
	}
	if len(view.ByWorkflow) != 1 || view.ByWorkflow[0].ModelCalls != 4 || view.ByWorkflow[0].FailedAgents != 1 || view.ByWorkflow[0].FailedJobs != 1 {
		t.Fatalf("unexpected workflow metrics: %#v", view.ByWorkflow)
	}
	if view.Summary.BackgroundJobFailed != 1 || view.Summary.BackgroundJobRetrying != 1 {
		t.Fatalf("unexpected job summary: %#v", view.Summary)
	}
}

func TestFilterRowsScopesByProviderWorkflowAndTask(t *testing.T) {
	now := time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC)
	taskID := 7
	otherTaskID := 8
	generateRun := 10
	analyzeRun := 11
	matchedAgent := 20
	otherAgent := 21

	filtered := filterRows(Input{
		Provider:     "openai",
		WorkflowType: models.JobTypeGenerate,
		TaskID:       taskID,
	}, rows{
		WorkflowRuns: []models.WorkflowRun{
			{ID: generateRun, WorkflowType: models.JobTypeGenerate, ResourceType: "task", ResourceID: taskID, CreatedAt: now},
			{ID: analyzeRun, WorkflowType: models.JobTypeAnalyze, ResourceType: "task", ResourceID: taskID, CreatedAt: now},
		},
		AgentRuns: []models.AgentRun{
			{ID: matchedAgent, WorkflowRunID: &generateRun, TaskID: &taskID, CreatedAt: now},
			{ID: otherAgent, WorkflowRunID: &analyzeRun, TaskID: &taskID, CreatedAt: now},
		},
		ModelCalls: []models.ModelCall{
			{ID: 30, WorkflowRunID: &generateRun, AgentRunID: &matchedAgent, Provider: "openai", Model: "gpt-5", CreatedAt: now},
			{ID: 31, WorkflowRunID: &generateRun, AgentRunID: &matchedAgent, Provider: "local", Model: "llama", CreatedAt: now},
			{ID: 32, WorkflowRunID: &analyzeRun, AgentRunID: &otherAgent, Provider: "openai", Model: "gpt-5", CreatedAt: now},
		},
		BackgroundJobs: []models.BackgroundJob{
			{ID: 40, WorkflowRunID: &generateRun, TaskID: &taskID, JobType: models.JobTypeGenerate, CreatedAt: now},
			{ID: 41, WorkflowRunID: &analyzeRun, TaskID: &taskID, JobType: models.JobTypeAnalyze, CreatedAt: now},
			{ID: 42, TaskID: &otherTaskID, JobType: models.JobTypeGenerate, CreatedAt: now},
		},
	})

	if len(filtered.WorkflowRuns) != 1 || filtered.WorkflowRuns[0].ID != generateRun {
		t.Fatalf("unexpected filtered workflows: %#v", filtered.WorkflowRuns)
	}
	if len(filtered.AgentRuns) != 1 || filtered.AgentRuns[0].ID != matchedAgent {
		t.Fatalf("unexpected filtered agents: %#v", filtered.AgentRuns)
	}
	if len(filtered.ModelCalls) != 1 || filtered.ModelCalls[0].ID != 30 {
		t.Fatalf("unexpected filtered model calls: %#v", filtered.ModelCalls)
	}
	if len(filtered.BackgroundJobs) != 1 || filtered.BackgroundJobs[0].ID != 40 {
		t.Fatalf("unexpected filtered jobs: %#v", filtered.BackgroundJobs)
	}
}
