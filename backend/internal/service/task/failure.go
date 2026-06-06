package task

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"caseagent/internal/db/models"
	agentservice "caseagent/internal/service/agent"
	suggestionservice "caseagent/internal/service/suggestion"
)

const (
	GenerationStageInitializeAgent = "initialize_agent"
	GenerationStageAgentGenerate   = "agent_generate"
	GenerationStageParseCases      = "parse_generated_cases"
	GenerationStageEmptyCases      = "empty_generated_cases"
)

type GenerationFailureError struct {
	Stage string
	Err   error
}

func (s *Service) MarkTaskFailed(ctx context.Context, taskID int) error {
	return s.updateTaskStatus(ctx, taskID, models.TaskStatusFailed)
}

func (s *Service) MarkGenerationFailed(ctx context.Context, taskID int, cause error) error {
	if err := s.MarkTaskFailed(ctx, taskID); err != nil {
		return err
	}
	if !ShouldRecordContextGap(cause) {
		return nil
	}
	if err := s.RecordGenerationFailureSuggestion(ctx, taskID, cause); err != nil {
		slog.Warn("generation failure suggestion record failed",
			"task_id", taskID, "stage", GenerationFailureStage(cause), "error", err)
	}
	return nil
}

func (s *Service) RecordGenerationFailureSuggestion(ctx context.Context, taskID int, cause error) error {
	task, err := s.getTask(ctx, taskID)
	if err != nil {
		return err
	}

	knowledgeEntries, err := s.loadRelevantKnowledge(ctx, task.AffectedProducts, task.AffectedModules)
	if err != nil {
		slog.Warn("failed to load relevant knowledge for generation failure suggestion",
			"task_id", taskID, "error", err)
		knowledgeEntries = nil
	}
	knowledgeIDs, knowledgeNames := summarizeKnowledgeEntries(knowledgeEntries)

	_, err = s.suggestionRecorder().RecordContextGap(ctx, suggestionservice.ContextGapInput{
		SourceTaskID:     taskID,
		FailureStage:     GenerationFailureStage(cause),
		ErrorSummary:     errorSummary(cause),
		AffectedProducts: append([]string{}, task.AffectedProducts...),
		AffectedModules:  append([]string{}, task.AffectedModules...),
		DocumentIDs:      append([]int{}, task.DocumentIDs...),
		KnowledgeIDs:     knowledgeIDs,
		KnowledgeNames:   knowledgeNames,
	})
	return err
}

func (e *GenerationFailureError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return fmt.Sprintf("%s: %v", e.Stage, e.Err)
}

func (e *GenerationFailureError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *GenerationFailureError) NonRetryable() bool {
	return e != nil && agentservice.IsTerminalGuardrailError(e.Err)
}

func generationFailure(stage string, format string, args ...any) error {
	return &GenerationFailureError{
		Stage: stage,
		Err:   fmt.Errorf(format, args...),
	}
}

func GenerationFailureStage(err error) string {
	var generationErr *GenerationFailureError
	if errors.As(err, &generationErr) {
		stage := strings.TrimSpace(generationErr.Stage)
		if stage != "" {
			return stage
		}
	}
	if stage := agentservice.FailureStage(err); stage != "" {
		return stage
	}
	return "generation"
}

func ShouldRecordContextGap(err error) bool {
	switch GenerationFailureStage(err) {
	case GenerationStageAgentGenerate,
		GenerationStageParseCases,
		GenerationStageEmptyCases,
		agentservice.GenerationStageDeepAgentFallback:
		return true
	default:
		return false
	}
}

func errorSummary(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func summarizeKnowledgeEntries(entries []models.KnowledgeBase) ([]int, []string) {
	ids := make([]int, 0, len(entries))
	names := make([]string, 0, len(entries))
	seenIDs := make(map[int]struct{}, len(entries))
	seenNames := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.ID > 0 {
			if _, ok := seenIDs[entry.ID]; !ok {
				seenIDs[entry.ID] = struct{}{}
				ids = append(ids, entry.ID)
			}
		}
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seenNames[key]; ok {
			continue
		}
		seenNames[key] = struct{}{}
		names = append(names, name)
	}
	return ids, names
}
