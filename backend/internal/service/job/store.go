package job

import (
	"context"
	"database/sql"
	"errors"
	"time"

	tenantdb "caseagent/internal/db"
	"caseagent/internal/db/models"
	workflowservice "caseagent/internal/service/workflow"

	"github.com/uptrace/bun"
)

type Store interface {
	ListTenantIDs(ctx context.Context) ([]int, error)
	RecoverStale(ctx context.Context, timeout time.Duration) (int, error)
	ClaimNext(ctx context.Context, tenantID int, jobType string) (*models.BackgroundJob, error)
	RunInTenantTx(ctx context.Context, tenantID int, fn func(context.Context, bun.Tx) error) error
	MarkSucceeded(ctx context.Context, tenantID int, jobID int) error
	MarkRetry(ctx context.Context, tenantID int, job *models.BackgroundJob, lastErr error, runAfter time.Time) error
	MarkFailed(ctx context.Context, tenantID int, jobID int, lastErr error) error
	StartWorkflow(ctx context.Context, job *models.BackgroundJob) (runID int, stepID int, err error)
	FinishWorkflow(ctx context.Context, tenantID int, runID int, stepID int, event workflowservice.TransitionEvent, cause error) error
}

type BunStore struct {
	db *bun.DB
}

func NewBunStore(db *bun.DB) *BunStore {
	return &BunStore{db: db}
}

func (s *BunStore) ListTenantIDs(ctx context.Context) ([]int, error) {
	var tenants []models.Tenant
	if err := s.db.NewSelect().
		Model(&tenants).
		Column("id").
		Order("id ASC").
		Scan(ctx); err != nil {
		return nil, err
	}

	ids := make([]int, 0, len(tenants))
	for _, tenant := range tenants {
		ids = append(ids, tenant.ID)
	}
	return ids, nil
}

func (s *BunStore) RecoverStale(ctx context.Context, timeout time.Duration) (int, error) {
	tenantIDs, err := s.ListTenantIDs(ctx)
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-timeout)
	recovered := 0
	for _, tenantID := range tenantIDs {
		if err := s.RunInTenantTx(ctx, tenantID, func(ctx context.Context, tx bun.Tx) error {
			result, err := tx.NewUpdate().
				Model((*models.BackgroundJob)(nil)).
				Set("status = ?", models.JobStatusPending).
				Set("locked_at = NULL").
				Set("run_after = CURRENT_TIMESTAMP").
				Set("updated_at = CURRENT_TIMESTAMP").
				Where("status = ?", models.JobStatusRunning).
				Where("(locked_at IS NULL OR locked_at < ?)", cutoff).
				Exec(ctx)
			if err != nil {
				return err
			}
			count, _ := result.RowsAffected()
			recovered += int(count)
			return nil
		}); err != nil {
			return recovered, err
		}
	}
	return recovered, nil
}

func (s *BunStore) ClaimNext(ctx context.Context, tenantID int, jobType string) (*models.BackgroundJob, error) {
	var claimed *models.BackgroundJob
	if err := s.RunInTenantTx(ctx, tenantID, func(ctx context.Context, tx bun.Tx) error {
		job := new(models.BackgroundJob)
		err := tx.NewRaw(`
WITH next_job AS (
    SELECT id
    FROM background_jobs
    WHERE status = ?
      AND run_after <= CURRENT_TIMESTAMP
      AND (? = '' OR job_type = ?)
    ORDER BY run_after ASC, id ASC
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
UPDATE background_jobs AS j
SET status = ?,
    locked_at = CURRENT_TIMESTAMP,
    started_at = COALESCE(j.started_at, CURRENT_TIMESTAMP),
    updated_at = CURRENT_TIMESTAMP
FROM next_job
WHERE j.id = next_job.id
RETURNING j.*
`, models.JobStatusPending, jobType, jobType, models.JobStatusRunning).Scan(ctx, job)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		claimed = job
		return nil
	}); err != nil {
		return nil, err
	}
	return claimed, nil
}

func (s *BunStore) RunInTenantTx(ctx context.Context, tenantID int, fn func(context.Context, bun.Tx) error) error {
	return tenantdb.RunInTenantTx(tenantdb.WithTenant(ctx, tenantID), s.db, fn)
}

func (s *BunStore) MarkSucceeded(ctx context.Context, tenantID int, jobID int) error {
	return s.RunInTenantTx(ctx, tenantID, func(ctx context.Context, tx bun.Tx) error {
		now := time.Now()
		_, err := tx.NewUpdate().
			Model((*models.BackgroundJob)(nil)).
			Set("status = ?", models.JobStatusSucceeded).
			Set("locked_at = NULL").
			Set("finished_at = ?", now).
			Set("updated_at = ?", now).
			Where("id = ?", jobID).
			Where("status = ?", models.JobStatusRunning).
			Exec(ctx)
		return err
	})
}

func (s *BunStore) MarkRetry(ctx context.Context, tenantID int, job *models.BackgroundJob, lastErr error, runAfter time.Time) error {
	return s.RunInTenantTx(ctx, tenantID, func(ctx context.Context, tx bun.Tx) error {
		now := time.Now()
		_, err := tx.NewUpdate().
			Model((*models.BackgroundJob)(nil)).
			Set("status = ?", models.JobStatusPending).
			Set("retry_count = ?", job.RetryCount+1).
			Set("last_error = ?", errorString(lastErr)).
			Set("run_after = ?", runAfter).
			Set("locked_at = NULL").
			Set("updated_at = ?", now).
			Where("id = ?", job.ID).
			Where("status = ?", models.JobStatusRunning).
			Exec(ctx)
		return err
	})
}

func (s *BunStore) MarkFailed(ctx context.Context, tenantID int, jobID int, lastErr error) error {
	return s.RunInTenantTx(ctx, tenantID, func(ctx context.Context, tx bun.Tx) error {
		now := time.Now()
		_, err := tx.NewUpdate().
			Model((*models.BackgroundJob)(nil)).
			Set("status = ?", models.JobStatusFailed).
			Set("last_error = ?", errorString(lastErr)).
			Set("locked_at = NULL").
			Set("finished_at = ?", now).
			Set("updated_at = ?", now).
			Where("id = ?", jobID).
			Where("status = ?", models.JobStatusRunning).
			Exec(ctx)
		return err
	})
}

func (s *BunStore) StartWorkflow(ctx context.Context, job *models.BackgroundJob) (int, int, error) {
	var runID int
	var stepID int
	if err := s.RunInTenantTx(ctx, job.TenantID, func(ctx context.Context, tx bun.Tx) error {
		run, step, err := workflowservice.New(tx).StartJobRun(ctx, workflowservice.StartJobRunInput{Job: job})
		if err != nil {
			return err
		}
		runID = run.ID
		stepID = step.ID
		return nil
	}); err != nil {
		return 0, 0, err
	}
	return runID, stepID, nil
}

func (s *BunStore) FinishWorkflow(ctx context.Context, tenantID int, runID int, stepID int, event workflowservice.TransitionEvent, cause error) error {
	return s.RunInTenantTx(ctx, tenantID, func(ctx context.Context, tx bun.Tx) error {
		return workflowservice.New(tx).FinishRunAndStep(ctx, runID, stepID, workflowservice.FinishInput{
			Event: event,
			Cause: cause,
		})
	})
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
