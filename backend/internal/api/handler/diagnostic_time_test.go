package handler

import (
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"caseagent/internal/db/models"
)

func TestDiagnosticTimestampsMarshalWithTimezoneAndRemainOrdered(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	created := time.Date(2026, 6, 22, 9, 0, 0, 0, location)
	started := created.Add(2 * time.Second)
	finished := started.Add(45 * time.Second)

	payload := struct {
		Task     models.CaseGenerationTask `json:"task"`
		Job      jobView                   `json:"job"`
		Workflow models.WorkflowRun        `json:"workflow"`
		Feedback models.TestCaseFeedback   `json:"feedback"`
	}{
		Task: models.CaseGenerationTask{CreatedAt: created, UpdatedAt: finished},
		Job: jobView{
			CreatedAt:  created,
			StartedAt:  &started,
			FinishedAt: &finished,
		},
		Workflow: models.WorkflowRun{CreatedAt: created, StartedAt: &started, FinishedAt: &finished},
		Feedback: models.TestCaseFeedback{CreatedAt: finished, UpdatedAt: finished},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal diagnostic payload: %v", err)
	}
	var decoded map[string]map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode diagnostic payload: %v", err)
	}

	timezoneSuffix := regexp.MustCompile(`(?:Z|[+-]\d{2}:\d{2})$`)
	for section, fields := range map[string][]string{
		"task":     {"created_at", "updated_at"},
		"job":      {"created_at", "started_at", "finished_at"},
		"workflow": {"created_at", "started_at", "finished_at"},
		"feedback": {"created_at", "updated_at"},
	} {
		for _, field := range fields {
			value, _ := decoded[section][field].(string)
			if !timezoneSuffix.MatchString(value) {
				t.Fatalf("%s.%s = %q, want RFC3339 timestamp with timezone", section, field, value)
			}
			if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
				t.Fatalf("parse %s.%s = %q: %v", section, field, value, err)
			}
		}
	}

	jobCreated := mustParseDiagnosticTime(t, decoded["job"]["created_at"])
	jobStarted := mustParseDiagnosticTime(t, decoded["job"]["started_at"])
	jobFinished := mustParseDiagnosticTime(t, decoded["job"]["finished_at"])
	if jobStarted.Before(jobCreated) || jobFinished.Before(jobStarted) {
		t.Fatalf("diagnostic timestamps out of order: created=%s started=%s finished=%s", jobCreated, jobStarted, jobFinished)
	}
}

func mustParseDiagnosticTime(t *testing.T, value any) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, value.(string))
	if err != nil {
		t.Fatalf("parse diagnostic time %q: %v", value, err)
	}
	return parsed
}
