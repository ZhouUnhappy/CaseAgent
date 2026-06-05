package job

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"caseagent/internal/db/models"

	"github.com/uptrace/bun"
)

const (
	defaultMaxConcurrency     = 2
	defaultMaxRetries         = 2
	defaultPollInterval       = 2 * time.Second
	defaultRetryBackoff       = 5 * time.Second
	defaultRunningJobTimeout  = 15 * time.Minute
	defaultStateUpdateTimeout = 10 * time.Second
)

type Executor interface {
	Execute(ctx context.Context, tx bun.Tx, job *models.CaseGenerationJob) error
	HandleFailure(ctx context.Context, tx bun.Tx, job *models.CaseGenerationJob, cause error) error
}

type Options struct {
	MaxConcurrency     int
	MaxRetries         int
	PollInterval       time.Duration
	RetryBackoff       time.Duration
	RunningJobTimeout  time.Duration
	StateUpdateTimeout time.Duration
}

type Runner struct {
	store    Store
	executor Executor
	options  Options
}

func NewRunner(store Store, executor Executor, options Options) *Runner {
	return &Runner{
		store:    store,
		executor: executor,
		options:  normalizeOptions(options),
	}
}

func (r *Runner) Start(ctx context.Context) {
	if recovered, err := r.store.RecoverStale(ctx, r.options.RunningJobTimeout); err != nil {
		slog.Error("case generation job recovery failed", "error", err)
	} else if recovered > 0 {
		slog.Info("case generation jobs recovered", "count", recovered)
	}

	for i := 0; i < r.options.MaxConcurrency; i++ {
		workerID := i + 1
		go r.worker(ctx, workerID)
	}
}

func (r *Runner) RunOne(ctx context.Context) (bool, error) {
	job, err := r.claimNext(ctx)
	if err != nil || job == nil {
		return false, err
	}

	r.process(ctx, job)
	return true, nil
}

func (r *Runner) worker(ctx context.Context, workerID int) {
	ticker := time.NewTicker(r.options.PollInterval)
	defer ticker.Stop()

	for {
		ran, err := r.RunOne(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("case generation worker failed",
				"worker_id", workerID,
				"error", err,
			)
		}
		if ran {
			continue
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Runner) claimNext(ctx context.Context) (*models.CaseGenerationJob, error) {
	tenantIDs, err := r.store.ListTenantIDs(ctx)
	if err != nil {
		return nil, err
	}

	for _, tenantID := range tenantIDs {
		job, err := r.store.ClaimNext(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		if job != nil {
			return job, nil
		}
	}
	return nil, nil
}

func (r *Runner) process(ctx context.Context, job *models.CaseGenerationJob) {
	slog.Info("case generation job started",
		"job_id", job.ID,
		"job_type", job.JobType,
		"tenant_id", job.TenantID,
		"task_id", job.TaskID,
		"retry_count", job.RetryCount,
		"max_retries", job.MaxRetries,
	)

	err := r.store.RunInTenantTx(ctx, job.TenantID, func(ctx context.Context, tx bun.Tx) error {
		return r.executor.Execute(ctx, tx, job)
	})
	if err == nil {
		if markErr := r.markSucceeded(job); markErr != nil {
			slog.Error("case generation job success mark failed",
				"job_id", job.ID,
				"tenant_id", job.TenantID,
				"error", markErr,
			)
		}
		return
	}
	if ctx.Err() != nil {
		slog.Warn("case generation job interrupted",
			"job_id", job.ID,
			"tenant_id", job.TenantID,
			"error", err,
		)
		return
	}

	if job.RetryCount < job.MaxRetries {
		nextRun := time.Now().Add(r.options.RetryBackoff)
		if markErr := r.markRetry(job, err, nextRun); markErr != nil {
			slog.Error("case generation job retry mark failed",
				"job_id", job.ID,
				"tenant_id", job.TenantID,
				"cause", err,
				"error", markErr,
			)
		}
		return
	}

	if failureErr := r.handleExhaustedFailure(job, err); failureErr != nil {
		slog.Error("case generation job failure handler failed",
			"job_id", job.ID,
			"tenant_id", job.TenantID,
			"cause", err,
			"error", failureErr,
		)
	}
	if markErr := r.markFailed(job, err); markErr != nil {
		slog.Error("case generation job failed mark failed",
			"job_id", job.ID,
			"tenant_id", job.TenantID,
			"cause", err,
			"error", markErr,
		)
	}
}

func (r *Runner) markSucceeded(job *models.CaseGenerationJob) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.options.StateUpdateTimeout)
	defer cancel()
	return r.store.MarkSucceeded(ctx, job.TenantID, job.ID)
}

func (r *Runner) markRetry(job *models.CaseGenerationJob, cause error, runAfter time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.options.StateUpdateTimeout)
	defer cancel()
	return r.store.MarkRetry(ctx, job.TenantID, job, cause, runAfter)
}

func (r *Runner) handleExhaustedFailure(job *models.CaseGenerationJob, cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.options.StateUpdateTimeout)
	defer cancel()
	return r.store.RunInTenantTx(ctx, job.TenantID, func(ctx context.Context, tx bun.Tx) error {
		return r.executor.HandleFailure(ctx, tx, job, cause)
	})
}

func (r *Runner) markFailed(job *models.CaseGenerationJob, cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.options.StateUpdateTimeout)
	defer cancel()
	return r.store.MarkFailed(ctx, job.TenantID, job.ID, cause)
}

func normalizeOptions(options Options) Options {
	if options.MaxConcurrency <= 0 {
		options.MaxConcurrency = defaultMaxConcurrency
	}
	if options.MaxRetries < 0 {
		options.MaxRetries = defaultMaxRetries
	}
	if options.PollInterval <= 0 {
		options.PollInterval = defaultPollInterval
	}
	if options.RetryBackoff < 0 {
		options.RetryBackoff = defaultRetryBackoff
	}
	if options.RunningJobTimeout <= 0 {
		options.RunningJobTimeout = defaultRunningJobTimeout
	}
	if options.StateUpdateTimeout <= 0 {
		options.StateUpdateTimeout = defaultStateUpdateTimeout
	}
	return options
}
