package job

import (
	"context"
	"fmt"
	"time"

	"caseagent/internal/db/models"
	documentservice "caseagent/internal/service/document"
	knowledgeservice "caseagent/internal/service/knowledge"
	taskservice "caseagent/internal/service/task"

	"github.com/uptrace/bun"
)

type TaskExecutor struct{}

func NewTaskExecutor() *TaskExecutor {
	return &TaskExecutor{}
}

func (e *TaskExecutor) Execute(ctx context.Context, tx bun.Tx, job *models.CaseGenerationJob) error {
	switch job.JobType {
	case models.JobTypeAnalyze:
		taskID, err := requiredResourceID(job.TaskID, "task_id")
		if err != nil {
			return err
		}
		return taskservice.New(tx).AnalyzeTask(ctx, taskID)
	case models.JobTypeGenerate:
		taskID, err := requiredResourceID(job.TaskID, "task_id")
		if err != nil {
			return err
		}
		return taskservice.New(tx).GenerateCases(ctx, taskID)
	case models.JobTypeDocumentProcess, models.JobTypeDocumentReprocess:
		docID, err := requiredResourceID(job.DocumentID, "document_id")
		if err != nil {
			return err
		}
		docService, err := documentservice.New(ctx, tx)
		if err != nil {
			return err
		}
		return docService.ReprocessDocument(ctx, docID)
	case models.JobTypeKnowledgeProcess, models.JobTypeKnowledgeReprocess:
		kbID, err := requiredResourceID(job.KnowledgeID, "knowledge_id")
		if err != nil {
			return err
		}
		knowledgeService, err := knowledgeservice.New(ctx, tx)
		if err != nil {
			return err
		}
		return knowledgeService.ReprocessKnowledge(ctx, kbID)
	default:
		return fmt.Errorf("unsupported background job type %q", job.JobType)
	}
}

func (e *TaskExecutor) HandleFailure(ctx context.Context, tx bun.Tx, job *models.CaseGenerationJob, cause error) error {
	switch job.JobType {
	case models.JobTypeAnalyze:
		taskID, err := requiredResourceID(job.TaskID, "task_id")
		if err != nil {
			return err
		}
		return taskservice.New(tx).MarkTaskFailed(ctx, taskID)
	case models.JobTypeGenerate:
		taskID, err := requiredResourceID(job.TaskID, "task_id")
		if err != nil {
			return err
		}
		return taskservice.New(tx).MarkGenerationFailed(ctx, taskID, cause)
	case models.JobTypeDocumentProcess, models.JobTypeDocumentReprocess:
		docID, err := requiredResourceID(job.DocumentID, "document_id")
		if err != nil {
			return err
		}
		_, err = tx.NewUpdate().Model(&models.Document{}).
			Set("status = ?", models.DocumentStatusFailed).
			Set("updated_at = ?", time.Now()).
			Where("id = ?", docID).
			Exec(ctx)
		return err
	case models.JobTypeKnowledgeProcess, models.JobTypeKnowledgeReprocess:
		kbID, err := requiredResourceID(job.KnowledgeID, "knowledge_id")
		if err != nil {
			return err
		}
		_, err = tx.NewUpdate().Model(&models.KnowledgeBase{}).
			Set("status = ?", models.KnowledgeStatusFailed).
			Set("updated_at = ?", time.Now()).
			Where("id = ?", kbID).
			Exec(ctx)
		return err
	default:
		return fmt.Errorf("unsupported background job type %q", job.JobType)
	}
}

func requiredResourceID(id *int, field string) (int, error) {
	if id == nil || *id <= 0 {
		return 0, fmt.Errorf("background job missing %s", field)
	}
	return *id, nil
}
