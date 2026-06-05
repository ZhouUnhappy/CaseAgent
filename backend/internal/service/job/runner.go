package job

import (
	"context"
	"errors"
	"log/slog"
	"sort"
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
	JobTypes           map[string]JobTypeOptions
}

type JobTypeOptions struct {
	MaxConcurrency int
	MaxRetries     int
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

	workerTypes := r.workerTypes()
	if len(workerTypes) == 0 {
		for i := 0; i < r.options.MaxConcurrency; i++ {
			workerID := i + 1
			go r.worker(ctx, workerID, "")
		}
		return
	}

	workerID := 0
	for _, jobType := range workerTypes {
		maxConcurrency := r.options.MaxConcurrency
		if typeOptions, ok := r.options.JobTypes[jobType]; ok && typeOptions.MaxConcurrency > 0 {
			maxConcurrency = typeOptions.MaxConcurrency
		}
		for i := 0; i < maxConcurrency; i++ {
			workerID++
			go r.worker(ctx, workerID, jobType)
		}
	}
}

func (r *Runner) workerTypes() []string {
	hasTypeConcurrency := false
	for _, typeOptions := range r.options.JobTypes {
		if typeOptions.MaxConcurrency > 0 {
			hasTypeConcurrency = true
			break
		}
	}
	if !hasTypeConcurrency {
		return nil
	}

	types := append([]string(nil), models.AllJobTypes...)
	sort.Strings(types)
	return types
}

func (r *Runner) RunOne(ctx context.Context) (bool, error) {
	return r.runOne(ctx, "")
}

func (r *Runner) runOne(ctx context.Context, jobType string) (bool, error) {
	job, err := r.claimNext(ctx, jobType)
	if err != nil || job == nil {
		return false, err
	}

	r.process(ctx, job)
	return true, nil
}

func (r *Runner) worker(ctx context.Context, workerID int, jobType string) {
	ticker := time.NewTicker(r.options.PollInterval)
	defer ticker.Stop()

	for {
		ran, err := r.runOne(ctx, jobType)
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("background job worker failed",
				"worker_id", workerID,
				"job_type", jobType,
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

func (r *Runner) claimNext(ctx context.Context, jobType string) (*models.CaseGenerationJob, error) {
	tenantIDs, err := r.store.ListTenantIDs(ctx)
	if err != nil {
		return nil, err
	}

	for _, tenantID := range tenantIDs {
		job, err := r.store.ClaimNext(ctx, tenantID, jobType)
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
	slog.Info("background job started",
		"job_id", job.ID,
		"job_type", job.JobType,
		"tenant_id", job.TenantID,
		"task_id", optionalLogID(job.TaskID),
		"document_id", optionalLogID(job.DocumentID),
		"knowledge_id", optionalLogID(job.KnowledgeID),
		"retry_count", job.RetryCount,
		"max_retries", job.MaxRetries,
	)

	err := r.store.RunInTenantTx(ctx, job.TenantID, func(ctx context.Context, tx bun.Tx) error {
		return r.executor.Execute(ctx, tx, job)
	})
	if err == nil {
		if markErr := r.markSucceeded(job); markErr != nil {
			slog.Error("background job success mark failed",
				"job_id", job.ID,
				"tenant_id", job.TenantID,
				"error", markErr,
			)
		}
		return
	}
	if ctx.Err() != nil {
		slog.Warn("background job interrupted",
			"job_id", job.ID,
			"tenant_id", job.TenantID,
			"error", err,
		)
		return
	}

	if job.RetryCount < job.MaxRetries {
		nextRun := time.Now().Add(r.options.RetryBackoff)
		if markErr := r.markRetry(job, err, nextRun); markErr != nil {
			slog.Error("background job retry mark failed",
				"job_id", job.ID,
				"tenant_id", job.TenantID,
				"cause", err,
				"error", markErr,
			)
		}
		return
	}

	if failureErr := r.handleExhaustedFailure(job, err); failureErr != nil {
		slog.Error("background job failure handler failed",
			"job_id", job.ID,
			"tenant_id", job.TenantID,
			"cause", err,
			"error", failureErr,
		)
	}
	if markErr := r.markFailed(job, err); markErr != nil {
		slog.Error("background job failed mark failed",
			"job_id", job.ID,
			"tenant_id", job.TenantID,
			"cause", err,
			"error", markErr,
		)
	}
}

func optionalLogID(id *int) any {
	if id == nil {
		return nil
	}
	return *id
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
	if options.JobTypes == nil {
		options.JobTypes = map[string]JobTypeOptions{}
	}
	return options
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
