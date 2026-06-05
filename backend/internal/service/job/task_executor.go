package job

import (
	"context"
	"fmt"

	"caseagent/internal/db/models"
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
		return taskservice.New(tx).AnalyzeTask(ctx, job.TaskID)
	case models.JobTypeGenerate:
		return taskservice.New(tx).GenerateCases(ctx, job.TaskID)
	default:
		return fmt.Errorf("unsupported case generation job type %q", job.JobType)
	}
}

func (e *TaskExecutor) HandleFailure(ctx context.Context, tx bun.Tx, job *models.CaseGenerationJob, cause error) error {
	switch job.JobType {
	case models.JobTypeAnalyze:
		return taskservice.New(tx).MarkTaskFailed(ctx, job.TaskID)
	case models.JobTypeGenerate:
		return taskservice.New(tx).MarkGenerationFailed(ctx, job.TaskID, cause)
	default:
		return fmt.Errorf("unsupported case generation job type %q", job.JobType)
	}
}
