package retention

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNormalizeInputValidation(t *testing.T) {
	_, err := normalizeInput(Input{RetentionDays: 0})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("normalizeInput() error = %v, want ErrInvalidInput", err)
	}

	_, err = normalizeInput(Input{RetentionDays: 7, Execute: true})
	if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "reason") {
		t.Fatalf("normalizeInput() error = %v, want execute reason error", err)
	}

	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	input, err := normalizeInput(Input{
		RetentionDays: 14,
		Now:           now,
		Execute:       true,
		OperatorID:    " qa-1 ",
		OperatorName:  " QA Lead ",
		Reason:        " old traces ",
	})
	if err != nil {
		t.Fatalf("normalizeInput() returned error: %v", err)
	}
	if input.OperatorID != "qa-1" || input.OperatorName != "QA Lead" || input.Reason != "old traces" {
		t.Fatalf("normalizeInput() did not trim operator fields: %#v", input)
	}
}

func TestCleanupTargetsDeleteChildrenBeforeWorkflowRuns(t *testing.T) {
	targets := cleanupTargets()
	got := make([]string, 0, len(targets))
	for _, target := range targets {
		got = append(got, target.Name)
	}

	want := []string{
		"test_case_feedback",
		"artifacts",
		"retrieval_runs",
		"model_calls",
		"agent_runs",
		"workflow_steps",
		"workflow_runs",
		"test_cases.source_context",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("cleanupTargets() = %v, want %v", got, want)
	}
}

func TestCandidateSQLPlaceholderCountsMatchArgs(t *testing.T) {
	cutoff := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	for _, target := range cleanupTargets() {
		sqlText, args, err := candidateWhereSQL(target.Name, 42, cutoff)
		if err != nil {
			t.Fatalf("candidateWhereSQL(%s) returned error: %v", target.Name, err)
		}
		if placeholders := strings.Count(sqlText, "?"); placeholders != len(args) {
			t.Fatalf("candidateWhereSQL(%s) placeholders=%d args=%d\n%s", target.Name, placeholders, len(args), sqlText)
		}
	}
}

func TestCleanupArtifactPayloadIncludesSummaryAndPreservation(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	report := &Report{
		GeneratedAt:   now,
		TenantID:      7,
		Mode:          ModeExecute,
		RetentionDays: 30,
		Cutoff:        now.AddDate(0, 0, -30),
		Before: []TargetStat{{
			Target:        "model_calls",
			Operation:     "delete",
			Rows:          9,
			BytesEstimate: 900,
		}},
		Candidates: []TargetStat{{
			Target:        "model_calls",
			Operation:     "delete",
			Rows:          3,
			BytesEstimate: 300,
		}},
		Deleted: []TargetStat{{
			Target:        "model_calls",
			Operation:     "delete",
			Rows:          3,
			BytesEstimate: 300,
		}},
		Preserved: defaultPreservation(true),
	}

	payload := cleanupArtifactPayload(Input{
		OperatorID:   "qa-1",
		OperatorName: "QA Lead",
		Reason:       "reduce trace bloat",
	}, report)

	if payload["operator_id"] != "qa-1" || payload["reason"] != "reduce trace bloat" {
		t.Fatalf("unexpected operator payload: %#v", payload)
	}
	deleted, ok := payload["deleted"].([]map[string]any)
	if !ok || len(deleted) != 1 || deleted[0]["target"] != "model_calls" || deleted[0]["rows"] != int64(3) {
		t.Fatalf("unexpected deleted payload: %#v", payload["deleted"])
	}
	preserved, ok := payload["preserved"].(Preservation)
	if !ok || !preserved.TaskFinalStatus || !preserved.InterventionArtifacts || !preserved.CleanupAuditArtifact {
		t.Fatalf("unexpected preservation payload: %#v", payload["preserved"])
	}
}
