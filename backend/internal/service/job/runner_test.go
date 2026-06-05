package job

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	tenantdb "caseagent/internal/db"
	"caseagent/internal/db/models"
	workflowservice "caseagent/internal/service/workflow"

	"github.com/uptrace/bun"
)

func TestRunOneRetriesThenSucceeds(t *testing.T) {
	store := newFakeStore([]int{1}, []*models.BackgroundJob{
		{
			ID:         10,
			TenantID:   1,
			TaskID:     testID(100),
			JobType:    models.JobTypeAnalyze,
			Status:     models.JobStatusPending,
			MaxRetries: 2,
			RunAfter:   time.Now().Add(-time.Second),
		},
	})
	executor := &scriptedExecutor{failuresLeft: 1, err: errors.New("temporary failure")}
	runner := NewRunner(store, executor, Options{RetryBackoff: 0})

	ran, err := runner.RunOne(context.Background())
	if err != nil || !ran {
		t.Fatalf("first RunOne ran=%v err=%v", ran, err)
	}
	job := store.job(10)
	if job.Status != models.JobStatusPending || job.RetryCount != 1 || job.LastError != "temporary failure" {
		t.Fatalf("job after first failure = %#v", job)
	}

	ran, err = runner.RunOne(context.Background())
	if err != nil || !ran {
		t.Fatalf("second RunOne ran=%v err=%v", ran, err)
	}
	job = store.job(10)
	if job.Status != models.JobStatusSucceeded {
		t.Fatalf("job status after retry = %q, want succeeded", job.Status)
	}
	if got := executor.executeTenantIDs(); len(got) != 2 || got[0] != 1 || got[1] != 1 {
		t.Fatalf("execute tenant ids = %#v", got)
	}
}

func TestRunOneExhaustsRetriesAndCallsFailureHandler(t *testing.T) {
	store := newFakeStore([]int{7}, []*models.BackgroundJob{
		{
			ID:         20,
			TenantID:   7,
			TaskID:     testID(200),
			JobType:    models.JobTypeGenerate,
			Status:     models.JobStatusPending,
			MaxRetries: 1,
			RunAfter:   time.Now().Add(-time.Second),
		},
	})
	executor := &scriptedExecutor{failuresLeft: 10, err: errors.New("provider unavailable")}
	runner := NewRunner(store, executor, Options{RetryBackoff: 0})

	for i := 0; i < 2; i++ {
		ran, err := runner.RunOne(context.Background())
		if err != nil || !ran {
			t.Fatalf("RunOne #%d ran=%v err=%v", i+1, ran, err)
		}
	}

	job := store.job(20)
	if job.Status != models.JobStatusFailed {
		t.Fatalf("job status = %q, want failed", job.Status)
	}
	if job.RetryCount != 1 {
		t.Fatalf("retry_count = %d, want 1", job.RetryCount)
	}
	if job.LastError != "provider unavailable" {
		t.Fatalf("last_error = %q", job.LastError)
	}
	if got := executor.failureTenantIDs(); len(got) != 1 || got[0] != 7 {
		t.Fatalf("failure handler tenant ids = %#v", got)
	}
}

func TestRunOneRecordsWorkflowAndPropagatesRunID(t *testing.T) {
	store := newFakeStore([]int{3}, []*models.BackgroundJob{
		{
			ID:         30,
			TenantID:   3,
			TaskID:     testID(300),
			JobType:    models.JobTypeGenerate,
			Status:     models.JobStatusPending,
			MaxRetries: 0,
			RunAfter:   time.Now().Add(-time.Second),
		},
	})
	executor := &scriptedExecutor{}
	runner := NewRunner(store, executor, Options{})

	ran, err := runner.RunOne(context.Background())
	if err != nil || !ran {
		t.Fatalf("RunOne ran=%v err=%v", ran, err)
	}

	starts := store.workflowStarts()
	if len(starts) != 1 {
		t.Fatalf("workflow starts = %#v, want one", starts)
	}
	if starts[0].jobID != 30 || starts[0].runID <= 0 || starts[0].stepID <= 0 {
		t.Fatalf("workflow start = %#v", starts[0])
	}
	if got := executor.executeRunIDs(); len(got) != 1 || got[0] != starts[0].runID {
		t.Fatalf("execute workflow run ids = %#v, want [%d]", got, starts[0].runID)
	}

	finishes := store.workflowFinishes()
	if len(finishes) != 1 {
		t.Fatalf("workflow finishes = %#v, want one", finishes)
	}
	if finishes[0].runID != starts[0].runID ||
		finishes[0].stepID != starts[0].stepID ||
		finishes[0].status != models.WorkflowStatusSucceeded {
		t.Fatalf("workflow finish = %#v, start = %#v", finishes[0], starts[0])
	}
}

func TestStartRecoversStaleRunningJobs(t *testing.T) {
	oldLock := time.Now().Add(-time.Hour)
	freshLock := time.Now()
	store := newFakeStore([]int{1}, []*models.BackgroundJob{
		{ID: 1, TenantID: 1, Status: models.JobStatusRunning, LockedAt: &oldLock},
		{ID: 2, TenantID: 1, Status: models.JobStatusRunning, LockedAt: &freshLock},
	})
	runner := NewRunner(store, &scriptedExecutor{}, Options{
		MaxConcurrency:    1,
		PollInterval:      time.Hour,
		RunningJobTimeout: time.Minute,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner.Start(ctx)

	if store.recoverCalls() != 1 {
		t.Fatalf("recover calls = %d, want 1", store.recoverCalls())
	}
	if got := store.job(1).Status; got != models.JobStatusPending {
		t.Fatalf("stale job status = %q, want pending", got)
	}
	if got := store.job(2).Status; got != models.JobStatusRunning {
		t.Fatalf("fresh job status = %q, want running", got)
	}
}

func TestStartHonorsMaxConcurrency(t *testing.T) {
	jobs := make([]*models.BackgroundJob, 0, 5)
	for i := 1; i <= 5; i++ {
		jobs = append(jobs, &models.BackgroundJob{
			ID:         i,
			TenantID:   1,
			TaskID:     testID(i),
			JobType:    models.JobTypeAnalyze,
			Status:     models.JobStatusPending,
			MaxRetries: 0,
			RunAfter:   time.Now().Add(-time.Second),
		})
	}
	store := newFakeStore([]int{1}, jobs)
	executor := newBlockingExecutor()
	runner := NewRunner(store, executor, Options{
		MaxConcurrency: 2,
		PollInterval:   10 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)

	executor.waitStarted(t, 2)
	time.Sleep(30 * time.Millisecond)
	if got := executor.maxActive(); got > 2 {
		t.Fatalf("max active executions = %d, want <= 2", got)
	}
	if got := store.runningCount(); got != 2 {
		t.Fatalf("running jobs = %d, want 2", got)
	}

	cancel()
	executor.releaseAll()
}

func TestStartHonorsJobTypeConcurrency(t *testing.T) {
	jobs := []*models.BackgroundJob{
		{
			ID:         1,
			TenantID:   1,
			TaskID:     testID(1),
			JobType:    models.JobTypeAnalyze,
			Status:     models.JobStatusPending,
			MaxRetries: 0,
			RunAfter:   time.Now().Add(-time.Second),
		},
		{
			ID:         2,
			TenantID:   1,
			TaskID:     testID(2),
			JobType:    models.JobTypeAnalyze,
			Status:     models.JobStatusPending,
			MaxRetries: 0,
			RunAfter:   time.Now().Add(-time.Second),
		},
		{
			ID:         3,
			TenantID:   1,
			DocumentID: testID(3),
			JobType:    models.JobTypeDocumentReprocess,
			Status:     models.JobStatusPending,
			MaxRetries: 0,
			RunAfter:   time.Now().Add(-time.Second),
		},
	}
	store := newFakeStore([]int{1}, jobs)
	executor := newBlockingExecutor()
	runner := NewRunner(store, executor, Options{
		MaxConcurrency: 1,
		PollInterval:   10 * time.Millisecond,
		JobTypes: map[string]JobTypeOptions{
			models.JobTypeAnalyze:           {MaxConcurrency: 1},
			models.JobTypeDocumentReprocess: {MaxConcurrency: 1},
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)

	executor.waitStarted(t, 2)
	time.Sleep(30 * time.Millisecond)
	if got := store.runningCountByType(models.JobTypeAnalyze); got != 1 {
		t.Fatalf("running analyze jobs = %d, want 1", got)
	}
	if got := store.runningCountByType(models.JobTypeDocumentReprocess); got != 1 {
		t.Fatalf("running document jobs = %d, want 1", got)
	}

	cancel()
	executor.releaseAll()
}

type fakeStore struct {
	mu                   sync.Mutex
	tenants              []int
	jobs                 []*models.BackgroundJob
	recoveries           int
	nextWorkflowRunID    int
	nextWorkflowStepID   int
	workflowStartEvents  []workflowStart
	workflowFinishEvents []workflowFinish
}

func newFakeStore(tenants []int, jobs []*models.BackgroundJob) *fakeStore {
	cloned := make([]*models.BackgroundJob, 0, len(jobs))
	for _, job := range jobs {
		cloned = append(cloned, cloneJob(job))
	}
	return &fakeStore{
		tenants:            append([]int{}, tenants...),
		jobs:               cloned,
		nextWorkflowRunID:  100,
		nextWorkflowStepID: 500,
	}
}

func (s *fakeStore) ListTenantIDs(ctx context.Context) ([]int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int{}, s.tenants...), nil
}

func (s *fakeStore) RecoverStale(ctx context.Context, timeout time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recoveries++
	cutoff := time.Now().Add(-timeout)
	count := 0
	for _, job := range s.jobs {
		if job.Status != models.JobStatusRunning {
			continue
		}
		if job.LockedAt == nil || job.LockedAt.Before(cutoff) {
			job.Status = models.JobStatusPending
			job.LockedAt = nil
			job.RunAfter = time.Now().Add(time.Hour)
			count++
		}
	}
	return count, nil
}

func (s *fakeStore) ClaimNext(ctx context.Context, tenantID int, jobType string) (*models.BackgroundJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, job := range s.jobs {
		if job.TenantID != tenantID || job.Status != models.JobStatusPending {
			continue
		}
		if jobType != "" && job.JobType != jobType {
			continue
		}
		if !job.RunAfter.IsZero() && job.RunAfter.After(now) {
			continue
		}
		lockedAt := now
		job.Status = models.JobStatusRunning
		job.LockedAt = &lockedAt
		if job.StartedAt == nil {
			startedAt := now
			job.StartedAt = &startedAt
		}
		return cloneJob(job), nil
	}
	return nil, nil
}

func (s *fakeStore) RunInTenantTx(ctx context.Context, tenantID int, fn func(context.Context, bun.Tx) error) error {
	return fn(tenantdb.WithTenant(ctx, tenantID), bun.Tx{})
}

func (s *fakeStore) MarkSucceeded(ctx context.Context, tenantID int, jobID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.mustJob(jobID)
	job.Status = models.JobStatusSucceeded
	job.LockedAt = nil
	finishedAt := time.Now()
	job.FinishedAt = &finishedAt
	return nil
}

func (s *fakeStore) MarkRetry(ctx context.Context, tenantID int, job *models.BackgroundJob, lastErr error, runAfter time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored := s.mustJob(job.ID)
	stored.Status = models.JobStatusPending
	stored.RetryCount = job.RetryCount + 1
	stored.LastError = lastErr.Error()
	stored.RunAfter = runAfter
	stored.LockedAt = nil
	return nil
}

func (s *fakeStore) MarkFailed(ctx context.Context, tenantID int, jobID int, lastErr error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.mustJob(jobID)
	job.Status = models.JobStatusFailed
	job.LastError = lastErr.Error()
	job.LockedAt = nil
	finishedAt := time.Now()
	job.FinishedAt = &finishedAt
	return nil
}

func (s *fakeStore) StartWorkflow(ctx context.Context, job *models.BackgroundJob) (int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	runID := s.nextWorkflowRunID
	stepID := s.nextWorkflowStepID
	s.nextWorkflowRunID++
	s.nextWorkflowStepID++
	s.workflowStartEvents = append(s.workflowStartEvents, workflowStart{
		jobID:  job.ID,
		runID:  runID,
		stepID: stepID,
	})
	return runID, stepID, nil
}

func (s *fakeStore) FinishWorkflow(ctx context.Context, tenantID int, runID int, stepID int, status string, cause error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lastErr := ""
	if cause != nil {
		lastErr = cause.Error()
	}
	s.workflowFinishEvents = append(s.workflowFinishEvents, workflowFinish{
		tenantID: tenantID,
		runID:    runID,
		stepID:   stepID,
		status:   status,
		lastErr:  lastErr,
	})
	return nil
}

type workflowStart struct {
	jobID  int
	runID  int
	stepID int
}

type workflowFinish struct {
	tenantID int
	runID    int
	stepID   int
	status   string
	lastErr  string
}

func (s *fakeStore) workflowStarts() []workflowStart {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]workflowStart{}, s.workflowStartEvents...)
}

func (s *fakeStore) workflowFinishes() []workflowFinish {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]workflowFinish{}, s.workflowFinishEvents...)
}

func (s *fakeStore) job(id int) *models.BackgroundJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneJob(s.mustJob(id))
}

func (s *fakeStore) runningCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, job := range s.jobs {
		if job.Status == models.JobStatusRunning {
			count++
		}
	}
	return count
}

func (s *fakeStore) runningCountByType(jobType string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, job := range s.jobs {
		if job.JobType == jobType && job.Status == models.JobStatusRunning {
			count++
		}
	}
	return count
}

func (s *fakeStore) recoverCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recoveries
}

func (s *fakeStore) mustJob(id int) *models.BackgroundJob {
	for _, job := range s.jobs {
		if job.ID == id {
			return job
		}
	}
	panic("missing fake job")
}

func cloneJob(job *models.BackgroundJob) *models.BackgroundJob {
	if job == nil {
		return nil
	}
	cloned := *job
	return &cloned
}

func testID(id int) *int {
	return &id
}

type scriptedExecutor struct {
	mu           sync.Mutex
	failuresLeft int
	err          error
	executeIDs   []int
	failureIDs   []int
	runIDs       []int
}

func (e *scriptedExecutor) Execute(ctx context.Context, tx bun.Tx, job *models.BackgroundJob) error {
	tenantID, _ := tenantdb.TenantFromContext(ctx)
	runID, _ := workflowservice.RunIDFromContext(ctx)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.executeIDs = append(e.executeIDs, tenantID)
	e.runIDs = append(e.runIDs, runID)
	if e.failuresLeft > 0 {
		e.failuresLeft--
		return e.err
	}
	return nil
}

func (e *scriptedExecutor) HandleFailure(ctx context.Context, tx bun.Tx, job *models.BackgroundJob, cause error) error {
	tenantID, _ := tenantdb.TenantFromContext(ctx)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.failureIDs = append(e.failureIDs, tenantID)
	return nil
}

func (e *scriptedExecutor) executeTenantIDs() []int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]int{}, e.executeIDs...)
}

func (e *scriptedExecutor) failureTenantIDs() []int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]int{}, e.failureIDs...)
}

func (e *scriptedExecutor) executeRunIDs() []int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]int{}, e.runIDs...)
}

type blockingExecutor struct {
	mu        sync.Mutex
	active    int
	max       int
	started   chan struct{}
	release   chan struct{}
	releaseMu sync.Once
}

func newBlockingExecutor() *blockingExecutor {
	return &blockingExecutor{
		started: make(chan struct{}, 10),
		release: make(chan struct{}),
	}
}

func (e *blockingExecutor) Execute(ctx context.Context, tx bun.Tx, job *models.BackgroundJob) error {
	e.mu.Lock()
	e.active++
	if e.active > e.max {
		e.max = e.active
	}
	e.mu.Unlock()
	e.started <- struct{}{}

	select {
	case <-ctx.Done():
	case <-e.release:
	}

	e.mu.Lock()
	e.active--
	e.mu.Unlock()
	return ctx.Err()
}

func (e *blockingExecutor) HandleFailure(ctx context.Context, tx bun.Tx, job *models.BackgroundJob, cause error) error {
	return nil
}

func (e *blockingExecutor) waitStarted(t *testing.T, want int) {
	t.Helper()
	for i := 0; i < want; i++ {
		select {
		case <-e.started:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for execution %d", i+1)
		}
	}
}

func (e *blockingExecutor) maxActive() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.max
}

func (e *blockingExecutor) releaseAll() {
	e.releaseMu.Do(func() {
		close(e.release)
	})
}
