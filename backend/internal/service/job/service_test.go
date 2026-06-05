package job

import (
	"strings"
	"testing"

	"caseagent/internal/db/models"
)

func TestValidateEnqueueInput(t *testing.T) {
	tests := []struct {
		name    string
		input   EnqueueInput
		wantErr string
	}{
		{
			name: "task job requires task id",
			input: EnqueueInput{
				JobType:    models.JobTypeAnalyze,
				DocumentID: 1,
				MaxRetries: 1,
			},
			wantErr: "requires only task_id",
		},
		{
			name: "document job requires document id",
			input: EnqueueInput{
				JobType:    models.JobTypeDocumentProcess,
				MaxRetries: 1,
			},
			wantErr: "requires only document_id",
		},
		{
			name: "knowledge job requires knowledge id",
			input: EnqueueInput{
				JobType:     models.JobTypeKnowledgeReprocess,
				KnowledgeID: 9,
				MaxRetries:  1,
			},
		},
		{
			name: "negative retries are rejected",
			input: EnqueueInput{
				JobType:    models.JobTypeGenerate,
				TaskID:     3,
				MaxRetries: -1,
			},
			wantErr: "max_retries",
		},
		{
			name: "unknown job type is rejected",
			input: EnqueueInput{
				JobType:    "unknown",
				TaskID:     3,
				MaxRetries: 1,
			},
			wantErr: "unsupported job type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEnqueueInput(tt.input)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateEnqueueInput() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateEnqueueInput() error = %v, want contains %q", err, tt.wantErr)
			}
		})
	}
}
