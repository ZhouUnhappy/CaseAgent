package handler

import (
	"testing"

	"caseagent/internal/db/models"
)

func TestToJobViewMapsRetrying(t *testing.T) {
	view := toJobView(models.CaseGenerationJob{
		Status:     models.JobStatusPending,
		RetryCount: 1,
	})

	if view.Status != "retrying" {
		t.Fatalf("status = %q, want retrying", view.Status)
	}
}

func TestToJobViewKeepsInitialPending(t *testing.T) {
	view := toJobView(models.CaseGenerationJob{
		Status: models.JobStatusPending,
	})

	if view.Status != models.JobStatusPending {
		t.Fatalf("status = %q, want pending", view.Status)
	}
}
