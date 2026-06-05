package workflow

import (
	"strings"
	"testing"

	"caseagent/internal/db/models"
)

func TestNextStatusAllowsExpectedWorkflowTransitions(t *testing.T) {
	tests := []struct {
		name    string
		current string
		event   TransitionEvent
		want    string
	}{
		{
			name:    "start pending workflow",
			current: models.WorkflowStatusPending,
			event:   TransitionStart,
			want:    models.WorkflowStatusRunning,
		},
		{
			name:    "succeed running workflow",
			current: models.WorkflowStatusRunning,
			event:   TransitionSucceed,
			want:    models.WorkflowStatusSucceeded,
		},
		{
			name:    "fail running workflow",
			current: models.WorkflowStatusRunning,
			event:   TransitionFail,
			want:    models.WorkflowStatusFailed,
		},
		{
			name:    "cancel running workflow",
			current: models.WorkflowStatusRunning,
			event:   TransitionCancel,
			want:    models.WorkflowStatusCanceled,
		},
		{
			name:    "replay failed workflow",
			current: models.WorkflowStatusFailed,
			event:   TransitionReplay,
			want:    models.WorkflowStatusPending,
		},
		{
			name:    "replay canceled workflow",
			current: models.WorkflowStatusCanceled,
			event:   TransitionReplay,
			want:    models.WorkflowStatusPending,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NextStatus(tt.current, tt.event)
			if err != nil {
				t.Fatalf("NextStatus() returned error: %v", err)
			}
			if got.From != tt.current || got.Event != tt.event || got.To != tt.want {
				t.Fatalf("NextStatus() = %#v, want from=%q event=%q to=%q", got, tt.current, tt.event, tt.want)
			}
		})
	}
}

func TestNextStatusRejectsIllegalWorkflowTransitions(t *testing.T) {
	tests := []struct {
		name    string
		current string
		event   TransitionEvent
	}{
		{
			name:    "cannot succeed pending workflow",
			current: models.WorkflowStatusPending,
			event:   TransitionSucceed,
		},
		{
			name:    "cannot fail completed workflow",
			current: models.WorkflowStatusSucceeded,
			event:   TransitionFail,
		},
		{
			name:    "cannot start failed workflow without replay",
			current: models.WorkflowStatusFailed,
			event:   TransitionStart,
		},
		{
			name:    "unknown stored status",
			current: "wedged",
			event:   TransitionStart,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NextStatus(tt.current, tt.event)
			if err == nil {
				t.Fatal("NextStatus() expected error")
			}
			if !strings.Contains(err.Error(), "illegal workflow") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestIsTerminal(t *testing.T) {
	for _, status := range []string{
		models.WorkflowStatusSucceeded,
		models.WorkflowStatusFailed,
		models.WorkflowStatusCanceled,
	} {
		if !IsTerminal(status) {
			t.Fatalf("IsTerminal(%q) = false, want true", status)
		}
	}
	for _, status := range []string{
		models.WorkflowStatusPending,
		models.WorkflowStatusRunning,
		"wedged",
	} {
		if IsTerminal(status) {
			t.Fatalf("IsTerminal(%q) = true, want false", status)
		}
	}
}
