package task

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"caseagent/internal/clock"
	"caseagent/internal/db"
	"caseagent/internal/db/models"

	"github.com/uptrace/bun"
)

type BadRequestError struct {
	Message string
}

func (e *BadRequestError) Error() string {
	return e.Message
}

type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}

type ConflictError struct {
	Message string
}

func (e *ConflictError) Error() string {
	return e.Message
}

type RetryDecision struct {
	Task         *models.CaseGenerationTask
	RerunAnalyze bool
}

func (s *Service) CreateTask(ctx context.Context, projectID int, documentIDs []int) (*models.CaseGenerationTask, error) {
	documentIDs = dedupeInts(documentIDs)
	if len(documentIDs) == 0 {
		return nil, &BadRequestError{Message: "At least one document is required"}
	}

	if err := s.validateTaskDocuments(ctx, projectID, documentIDs); err != nil {
		return nil, err
	}

	tenantID, _ := db.TenantFromContext(ctx)
	now := clock.Now()
	task := &models.CaseGenerationTask{
		TenantID:    tenantID,
		ProjectID:   projectID,
		DocumentIDs: documentIDs,
		Status:      models.TaskStatusAnalyzing,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if _, err := s.db.NewInsert().Model(task).Exec(ctx); err != nil {
		return nil, err
	}

	return task, nil
}

func (s *Service) ReviewAffected(ctx context.Context, taskID int, products []string, modules []string) (*models.CaseGenerationTask, error) {
	task, err := s.getTask(ctx, taskID)
	if err != nil {
		return nil, mapTaskLoadError(err)
	}
	if !canReviewAffected(task.Status) {
		return nil, &ConflictError{
			Message: fmt.Sprintf("task status %q does not allow affected-scope review", task.Status),
		}
	}

	task.AffectedProducts = products
	task.AffectedModules = modules
	task.Status = models.TaskStatusReadyToGenerate
	task.UpdatedAt = clock.Now()

	if _, err := s.db.NewUpdate().Model(task).Where("id = ?", taskID).Exec(ctx); err != nil {
		return nil, err
	}

	return task, nil
}

func (s *Service) StartGeneration(ctx context.Context, taskID int) (*models.CaseGenerationTask, error) {
	task, err := s.getTask(ctx, taskID)
	if err != nil {
		return nil, mapTaskLoadError(err)
	}
	if !canStartGeneration(task.Status) {
		return nil, &ConflictError{
			Message: fmt.Sprintf("task status %q does not allow generation", task.Status),
		}
	}

	task.Status = models.TaskStatusGenerating
	task.UpdatedAt = clock.Now()

	updateResult, err := s.db.NewUpdate().
		Model(task).
		Where("id = ?", taskID).
		Where("status = ?", models.TaskStatusReadyToGenerate).
		Exec(ctx)
	if err != nil {
		return nil, err
	}
	if affected, _ := updateResult.RowsAffected(); affected == 0 {
		return nil, &ConflictError{Message: "task status has changed, please retry"}
	}

	return task, nil
}

func (s *Service) RetryTask(ctx context.Context, taskID int) (*RetryDecision, error) {
	task, err := s.getTask(ctx, taskID)
	if err != nil {
		return nil, mapTaskLoadError(err)
	}
	if task.Status != models.TaskStatusFailed {
		return nil, &ConflictError{
			Message: fmt.Sprintf("task status %q is not retryable; only failed tasks can be retried", task.Status),
		}
	}

	rerunAnalyze := len(task.AffectedProducts) == 0 && len(task.AffectedModules) == 0
	if rerunAnalyze {
		task.Status = models.TaskStatusAnalyzing
	} else {
		task.Status = models.TaskStatusReadyToGenerate
	}
	task.UpdatedAt = clock.Now()

	if _, err := s.db.NewUpdate().Model(task).
		Set("status = ?", task.Status).
		Set("updated_at = ?", task.UpdatedAt).
		Where("id = ?", taskID).
		Exec(ctx); err != nil {
		return nil, err
	}

	return &RetryDecision{Task: task, RerunAnalyze: rerunAnalyze}, nil
}

func (s *Service) validateTaskDocuments(ctx context.Context, projectID int, documentIDs []int) error {
	projectCount, err := s.db.NewSelect().Model((*models.Project)(nil)).Where("id = ?", projectID).Count(ctx)
	if err != nil {
		return fmt.Errorf("failed to verify project: %w", err)
	}
	if projectCount == 0 {
		return &BadRequestError{Message: "project not found"}
	}

	var documents []models.Document
	if err := s.db.NewSelect().
		Model(&documents).
		Where("project_id = ?", projectID).
		Where("id IN (?)", bun.In(documentIDs)).
		Scan(ctx); err != nil {
		return fmt.Errorf("failed to verify documents: %w", err)
	}

	if len(documents) != len(documentIDs) {
		return &BadRequestError{Message: "some documents do not belong to the project"}
	}

	for _, document := range documents {
		if document.Status != models.DocumentStatusCompleted {
			return &BadRequestError{Message: fmt.Sprintf("document %d is not ready, current status: %s", document.ID, document.Status)}
		}
	}

	return nil
}

func canReviewAffected(status string) bool {
	switch status {
	case models.TaskStatusAwaitingReview, models.TaskStatusReadyToGenerate:
		return true
	default:
		return false
	}
}

func canStartGeneration(status string) bool {
	return status == models.TaskStatusReadyToGenerate
}

func dedupeInts(values []int) []int {
	seen := make(map[int]struct{}, len(values))
	result := make([]int, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func mapTaskLoadError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return &NotFoundError{Message: "Task not found"}
	}
	return err
}
