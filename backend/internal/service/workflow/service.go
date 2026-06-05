package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	tenantdb "caseagent/internal/db"
	"caseagent/internal/db/models"

	"github.com/uptrace/bun"
)

type contextKey string

const runIDContextKey contextKey = "workflow_run_id"

type Service struct {
	db bun.IDB
}

type StartJobRunInput struct {
	Job *models.BackgroundJob
}

type FinishInput struct {
	Status string
	Cause  error
}

type AgentRunInput struct {
	WorkflowRunID *int
	TaskID        *int
	AgentName     string
	Stage         string
	Status        string
	InputSummary  string
	OutputSummary string
	LastError     string
	Metadata      map[string]any
}

type RetrievalRunInput struct {
	WorkflowRunID *int
	TaskID        *int
	RetrieverType string
	QueryCount    int
	HitCount      int
	Metadata      map[string]any
}

type ModelCallInput struct {
	WorkflowRunID *int
	AgentRunID    *int
	Provider      string
	Model         string
	Status        string
	PromptChars   int
	ResponseChars int
	LastError     string
	Metadata      map[string]any
}

type ArtifactInput struct {
	WorkflowRunID  *int
	WorkflowStepID *int
	ArtifactType   string
	ResourceType   string
	ResourceID     *int
	Name           string
	Content        string
	Payload        map[string]any
}

func New(db bun.IDB) *Service {
	return &Service{db: db}
}

func WithRunID(ctx context.Context, runID int) context.Context {
	if runID <= 0 {
		return ctx
	}
	return context.WithValue(ctx, runIDContextKey, runID)
}

func RunIDFromContext(ctx context.Context) (int, bool) {
	value, ok := ctx.Value(runIDContextKey).(int)
	return value, ok && value > 0
}

func RunIDPointerFromContext(ctx context.Context) *int {
	if id, ok := RunIDFromContext(ctx); ok {
		return &id
	}
	return nil
}

func (s *Service) StartJobRun(ctx context.Context, input StartJobRunInput) (*models.WorkflowRun, *models.WorkflowStep, error) {
	if input.Job == nil {
		return nil, nil, fmt.Errorf("start workflow: job is required")
	}
	tenantID, ok := tenantdb.TenantFromContext(ctx)
	if !ok {
		return nil, nil, fmt.Errorf("start workflow: no tenant in context")
	}

	resourceType, resourceID := jobResource(input.Job)
	if resourceID <= 0 {
		return nil, nil, fmt.Errorf("start workflow: job %d has no resource id", input.Job.ID)
	}

	now := time.Now()
	run := &models.WorkflowRun{
		TenantID:     tenantID,
		WorkflowType: input.Job.JobType,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		JobID:        &input.Job.ID,
		Status:       models.WorkflowStatusRunning,
		Metadata:     map[string]any{"retry_count": input.Job.RetryCount, "max_retries": input.Job.MaxRetries},
		StartedAt:    &now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if _, err := s.db.NewInsert().Model(run).Exec(ctx); err != nil {
		return nil, nil, err
	}

	step := &models.WorkflowStep{
		TenantID:      tenantID,
		WorkflowRunID: run.ID,
		StepType:      input.Job.JobType,
		Status:        models.WorkflowStatusRunning,
		Metadata:      map[string]any{},
		StartedAt:     &now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if _, err := s.db.NewInsert().Model(step).Exec(ctx); err != nil {
		return nil, nil, err
	}

	if _, err := s.db.NewUpdate().
		Model((*models.BackgroundJob)(nil)).
		Set("workflow_run_id = ?", run.ID).
		Set("updated_at = ?", now).
		Where("id = ?", input.Job.ID).
		Exec(ctx); err != nil {
		return nil, nil, err
	}

	return run, step, nil
}

func (s *Service) FinishRunAndStep(ctx context.Context, runID int, stepID int, input FinishInput) error {
	if runID <= 0 {
		return nil
	}
	status := normalizeStatus(input.Status)
	now := time.Now()
	if stepID > 0 {
		if _, err := s.db.NewUpdate().
			Model((*models.WorkflowStep)(nil)).
			Set("status = ?", status).
			Set("last_error = ?", errorString(input.Cause)).
			Set("finished_at = ?", now).
			Set("updated_at = ?", now).
			Where("id = ?", stepID).
			Exec(ctx); err != nil {
			return err
		}
	}
	_, err := s.db.NewUpdate().
		Model((*models.WorkflowRun)(nil)).
		Set("status = ?", status).
		Set("last_error = ?", errorString(input.Cause)).
		Set("finished_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", runID).
		Exec(ctx)
	return err
}

func (s *Service) RecordAgentRun(ctx context.Context, input AgentRunInput) (*models.AgentRun, error) {
	tenantID, ok := tenantdb.TenantFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("record agent run: no tenant in context")
	}
	now := time.Now()
	startedAt := now
	finishedAt := now
	row := &models.AgentRun{
		TenantID:      tenantID,
		WorkflowRunID: input.WorkflowRunID,
		TaskID:        input.TaskID,
		AgentName:     input.AgentName,
		Stage:         input.Stage,
		Status:        normalizeStatus(input.Status),
		InputSummary:  truncate(input.InputSummary, 2000),
		OutputSummary: truncate(input.OutputSummary, 2000),
		LastError:     truncate(input.LastError, 2000),
		Metadata:      defaultMap(input.Metadata),
		StartedAt:     &startedAt,
		FinishedAt:    &finishedAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if _, err := s.db.NewInsert().Model(row).Exec(ctx); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *Service) RecordRetrievalRun(ctx context.Context, input RetrievalRunInput) (*models.RetrievalRun, error) {
	tenantID, ok := tenantdb.TenantFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("record retrieval run: no tenant in context")
	}
	now := time.Now()
	startedAt := now
	finishedAt := now
	row := &models.RetrievalRun{
		TenantID:      tenantID,
		WorkflowRunID: input.WorkflowRunID,
		TaskID:        input.TaskID,
		RetrieverType: input.RetrieverType,
		Status:        models.WorkflowStatusSucceeded,
		QueryCount:    input.QueryCount,
		HitCount:      input.HitCount,
		Metadata:      defaultMap(input.Metadata),
		StartedAt:     &startedAt,
		FinishedAt:    &finishedAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if _, err := s.db.NewInsert().Model(row).Exec(ctx); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *Service) RecordModelCall(ctx context.Context, input ModelCallInput) (*models.ModelCall, error) {
	tenantID, ok := tenantdb.TenantFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("record model call: no tenant in context")
	}
	now := time.Now()
	startedAt := now
	finishedAt := now
	row := &models.ModelCall{
		TenantID:      tenantID,
		WorkflowRunID: input.WorkflowRunID,
		AgentRunID:    input.AgentRunID,
		Provider:      input.Provider,
		Model:         input.Model,
		Status:        normalizeStatus(input.Status),
		PromptChars:   nonNegative(input.PromptChars),
		ResponseChars: nonNegative(input.ResponseChars),
		LastError:     truncate(input.LastError, 2000),
		Metadata:      defaultMap(input.Metadata),
		StartedAt:     &startedAt,
		FinishedAt:    &finishedAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if _, err := s.db.NewInsert().Model(row).Exec(ctx); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *Service) RecordArtifact(ctx context.Context, input ArtifactInput) (*models.Artifact, error) {
	tenantID, ok := tenantdb.TenantFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("record artifact: no tenant in context")
	}
	row := &models.Artifact{
		TenantID:       tenantID,
		WorkflowRunID:  input.WorkflowRunID,
		WorkflowStepID: input.WorkflowStepID,
		ArtifactType:   input.ArtifactType,
		ResourceType:   input.ResourceType,
		ResourceID:     input.ResourceID,
		Name:           input.Name,
		Content:        input.Content,
		Payload:        defaultMap(input.Payload),
		CreatedAt:      time.Now(),
	}
	if _, err := s.db.NewInsert().Model(row).Exec(ctx); err != nil {
		return nil, err
	}
	return row, nil
}

func jobResource(job *models.BackgroundJob) (string, int) {
	if job.TaskID != nil {
		return "task", *job.TaskID
	}
	if job.DocumentID != nil {
		return "document", *job.DocumentID
	}
	if job.KnowledgeID != nil {
		return "knowledge", *job.KnowledgeID
	}
	return "", 0
}

func normalizeStatus(status string) string {
	switch status {
	case models.WorkflowStatusPending,
		models.WorkflowStatusRunning,
		models.WorkflowStatusSucceeded,
		models.WorkflowStatusFailed,
		models.WorkflowStatusCanceled:
		return status
	default:
		return models.WorkflowStatusSucceeded
	}
}

func defaultMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}

func nonNegative(value int) int {
	if value < 0 {
		return 0
	}
	return value
}
