package handler

import (
	"testing"

	"caseagent/internal/db/models"
)

func TestManualSuggestionInputValidation(t *testing.T) {
	valid := createSuggestionRequest{
		CandidateType: models.SuggestionCandidateModule,
		CandidateName: "Billing-Core",
		SourceTaskID:  10,
		SourceCaseID:  20,
	}

	if err := validateManualSuggestionRequest(valid); err != nil {
		t.Fatalf("valid request returned error: %v", err)
	}

	tests := []struct {
		name string
		req  createSuggestionRequest
	}{
		{
			name: "missing source_case_id",
			req: createSuggestionRequest{
				CandidateType: models.SuggestionCandidateModule,
				CandidateName: "Billing-Core",
				SourceTaskID:  10,
			},
		},
		{
			name: "missing source_task_id",
			req: createSuggestionRequest{
				CandidateType: models.SuggestionCandidateModule,
				CandidateName: "Billing-Core",
				SourceCaseID:  20,
			},
		},
		{
			name: "invalid candidate_type",
			req: createSuggestionRequest{
				CandidateType: "context_gap",
				CandidateName: "Billing-Core",
				SourceTaskID:  10,
				SourceCaseID:  20,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateManualSuggestionRequest(tc.req); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
