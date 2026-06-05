package handler

import (
	"testing"

	"caseagent/internal/db/models"
)

func TestToJobViewMapsRetrying(t *testing.T) {
	view := toJobView(models.BackgroundJob{
		Status:     models.JobStatusPending,
		RetryCount: 1,
	})

	if view.Status != "retrying" {
		t.Fatalf("status = %q, want retrying", view.Status)
	}
}

func TestToJobViewKeepsInitialPending(t *testing.T) {
	view := toJobView(models.BackgroundJob{
		Status: models.JobStatusPending,
	})

	if view.Status != models.JobStatusPending {
		t.Fatalf("status = %q, want pending", view.Status)
	}
}

func TestToJobViewKeepsCanceled(t *testing.T) {
	view := toJobView(models.BackgroundJob{
		Status: models.JobStatusCanceled,
	})

	if view.Status != models.JobStatusCanceled {
		t.Fatalf("status = %q, want canceled", view.Status)
	}
}

func TestJobActionGuards(t *testing.T) {
	if !canRetryJob(models.JobStatusFailed) || !canRetryJob(models.JobStatusCanceled) {
		t.Fatal("failed/canceled jobs should be retryable")
	}
	if canRetryJob(models.JobStatusRunning) {
		t.Fatal("running jobs should not be retryable")
	}
	if !canCancelJob(models.JobStatusPending) || !canCancelJob(models.JobStatusRunning) {
		t.Fatal("pending/running jobs should be cancelable")
	}
	if canCancelJob(models.JobStatusSucceeded) {
		t.Fatal("succeeded jobs should not be cancelable")
	}
	if !canReplayJob(models.JobStatusSucceeded) || !canReplayJob(models.JobStatusFailed) || !canReplayJob(models.JobStatusCanceled) {
		t.Fatal("terminal jobs should be replayable")
	}
	if canReplayJob(models.JobStatusPending) {
		t.Fatal("active jobs should not be replayable")
	}
}
