package task

import (
	"testing"

	"caseagent/internal/db/models"
)

func TestCanReviewAffected(t *testing.T) {
	if !canReviewAffected(models.TaskStatusAwaitingReview) {
		t.Fatal("awaiting_review should allow review")
	}
	if !canReviewAffected(models.TaskStatusReadyToGenerate) {
		t.Fatal("ready_to_generate should allow review update")
	}

	blocked := []string{
		models.TaskStatusAnalyzing,
		models.TaskStatusGenerating,
		models.TaskStatusCompleted,
		models.TaskStatusFailed,
	}
	for _, status := range blocked {
		if canReviewAffected(status) {
			t.Fatalf("status %q should not allow review", status)
		}
	}
}

func TestCanStartGeneration(t *testing.T) {
	if !canStartGeneration(models.TaskStatusReadyToGenerate) {
		t.Fatal("ready_to_generate should allow generation")
	}

	blocked := []string{
		models.TaskStatusAnalyzing,
		models.TaskStatusAwaitingReview,
		models.TaskStatusGenerating,
		models.TaskStatusCompleted,
		models.TaskStatusFailed,
	}
	for _, status := range blocked {
		if canStartGeneration(status) {
			t.Fatalf("status %q should not allow generation", status)
		}
	}
}
