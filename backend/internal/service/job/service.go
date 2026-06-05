package job

import (
	"context"
	"fmt"
	"time"

	tenantdb "caseagent/internal/db"
	"caseagent/internal/db/models"

	"github.com/uptrace/bun"
)

type Service struct {
	db bun.IDB
}

type EnqueueInput struct {
	TaskID     int
	JobType    string
	MaxRetries int
}

func New(db bun.IDB) *Service {
	return &Service{db: db}
}

func (s *Service) Enqueue(ctx context.Context, input EnqueueInput) (*models.CaseGenerationJob, error) {
	tenantID, ok := tenantdb.TenantFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("enqueue job: no tenant in context")
	}
	if input.TaskID <= 0 {
		return nil, fmt.Errorf("enqueue job: task_id is required")
	}
	if !validJobType(input.JobType) {
		return nil, fmt.Errorf("enqueue job: unsupported job type %q", input.JobType)
	}
	if input.MaxRetries < 0 {
		return nil, fmt.Errorf("enqueue job: max_retries must be >= 0")
	}

	now := time.Now()
	job := &models.CaseGenerationJob{
		TenantID:   tenantID,
		TaskID:     input.TaskID,
		JobType:    input.JobType,
		Status:     models.JobStatusPending,
		MaxRetries: input.MaxRetries,
		RunAfter:   now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if _, err := s.db.NewInsert().Model(job).Exec(ctx); err != nil {
		return nil, err
	}
	return job, nil
}

func validJobType(jobType string) bool {
	switch jobType {
	case models.JobTypeAnalyze, models.JobTypeGenerate:
		return true
	default:
		return false
	}
}
