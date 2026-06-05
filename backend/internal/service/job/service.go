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
	TaskID      int
	DocumentID  int
	KnowledgeID int
	JobType     string
	MaxRetries  int
	Payload     map[string]any
}

func New(db bun.IDB) *Service {
	return &Service{db: db}
}

func (s *Service) Enqueue(ctx context.Context, input EnqueueInput) (*models.CaseGenerationJob, error) {
	tenantID, ok := tenantdb.TenantFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("enqueue job: no tenant in context")
	}
	if err := validateEnqueueInput(input); err != nil {
		return nil, err
	}

	now := time.Now()
	job := &models.CaseGenerationJob{
		TenantID:    tenantID,
		TaskID:      optionalID(input.TaskID),
		DocumentID:  optionalID(input.DocumentID),
		KnowledgeID: optionalID(input.KnowledgeID),
		JobType:     input.JobType,
		Payload:     input.Payload,
		Status:      models.JobStatusPending,
		MaxRetries:  input.MaxRetries,
		RunAfter:    now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if job.Payload == nil {
		job.Payload = map[string]any{}
	}
	if _, err := s.db.NewInsert().Model(job).Exec(ctx); err != nil {
		return nil, err
	}
	return job, nil
}

func validateEnqueueInput(input EnqueueInput) error {
	if !validJobType(input.JobType) {
		return fmt.Errorf("enqueue job: unsupported job type %q", input.JobType)
	}
	if input.MaxRetries < 0 {
		return fmt.Errorf("enqueue job: max_retries must be >= 0")
	}

	switch input.JobType {
	case models.JobTypeAnalyze, models.JobTypeGenerate:
		if input.TaskID <= 0 || input.DocumentID != 0 || input.KnowledgeID != 0 {
			return fmt.Errorf("enqueue job: %s requires only task_id", input.JobType)
		}
	case models.JobTypeDocumentProcess, models.JobTypeDocumentReprocess:
		if input.DocumentID <= 0 || input.TaskID != 0 || input.KnowledgeID != 0 {
			return fmt.Errorf("enqueue job: %s requires only document_id", input.JobType)
		}
	case models.JobTypeKnowledgeProcess, models.JobTypeKnowledgeReprocess:
		if input.KnowledgeID <= 0 || input.TaskID != 0 || input.DocumentID != 0 {
			return fmt.Errorf("enqueue job: %s requires only knowledge_id", input.JobType)
		}
	default:
		return fmt.Errorf("enqueue job: unsupported job type %q", input.JobType)
	}
	return nil
}

func validJobType(jobType string) bool {
	for _, known := range models.AllJobTypes {
		if jobType == known {
			return true
		}
	}
	return false
}

func optionalID(id int) *int {
	if id <= 0 {
		return nil
	}
	return &id
}
