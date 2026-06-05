package task

import (
	"context"
	"log/slog"

	"caseagent/internal/db/models"
	agentservice "caseagent/internal/service/agent"

	"github.com/uptrace/bun"
)

type Service struct {
	db                    bun.IDB
	newCaseGenerator      func(ctx context.Context) (CaseGenerator, error)
	newRetriever          func() Retriever
	newSuggestionRecorder func() SuggestionRecorder
}

func New(db bun.IDB) *Service {
	return &Service{db: db}
}

func (s *Service) AnalyzeTask(ctx context.Context, taskID int) (err error) {
	defer func() {
		if err != nil {
			_ = s.updateTaskStatus(ctx, taskID, models.TaskStatusFailed)
		}
	}()

	task, err := s.getTask(ctx, taskID)
	if err != nil {
		return err
	}

	requirements, err := s.loadRequirements(ctx, task.ProjectID, task.DocumentIDs)
	if err != nil {
		return err
	}

	knowledgeEntries, err := s.listKnowledge(ctx)
	if err != nil {
		return err
	}

	products, modules := inferAffectedKnowledge(requirements, knowledgeEntries)
	if len(products) == 0 && len(modules) == 0 {
		products, modules = inferAffectedKnowledgeWithRetrieval(ctx, s.retriever(), requirements)
	}

	if err := s.updateTaskAnalysis(ctx, taskID, products, modules, models.TaskStatusAwaitingReview); err != nil {
		return err
	}

	// Knowledge gap suggestions are best-effort: failures must never break
	// AnalyzeTask. Run synchronously inside the current tenant tx — the bg
	// goroutine pattern broke when service moved to tx-scoped IDB (the tx
	// would be closed by the time the goroutine ran).
	if err := s.suggestionRecorder().RecordCandidates(ctx, taskID, requirements, products, modules); err != nil {
		slog.Warn("knowledge suggestion record failed",
			"task_id", taskID, "error", err)
	}

	return nil
}

func (s *Service) GenerateCases(ctx context.Context, taskID int) (err error) {
	if err = s.updateTaskStatus(ctx, taskID, models.TaskStatusGenerating); err != nil {
		return err
	}

	defer func() {
		if err != nil {
			_ = s.updateTaskStatus(ctx, taskID, models.TaskStatusFailed)
		}
	}()

	task, err := s.getTask(ctx, taskID)
	if err != nil {
		return err
	}

	requirements, err := s.loadRequirements(ctx, task.ProjectID, task.DocumentIDs)
	if err != nil {
		return err
	}

	knowledgeEntries, err := s.loadRelevantKnowledge(ctx, task.AffectedProducts, task.AffectedModules)
	if err != nil {
		return err
	}
	retriever := s.retriever()
	retrievedHits := retrieveKnowledgeFallback(ctx, retriever, requirements, task.AffectedProducts, task.AffectedModules)
	knowledgeEntries = mergeKnowledgeEntries(knowledgeEntries, knowledgeResultsToBaseEntries(retrievedHits))
	requirementsContext, documentHits := buildRequirementsContext(ctx, retriever, requirements, task.DocumentIDs, task.AffectedProducts, task.AffectedModules)

	agentSvc, err := s.caseGenerator(ctx)
	if err != nil {
		return generationFailure(GenerationStageInitializeAgent, "failed to initialize agent service: %w", err)
	}

	rawCases, err := agentSvc.GenerateCases(ctx, requirementsContext, formatKnowledgeContext(knowledgeEntries))
	if err != nil {
		stage := agentservice.FailureStage(err)
		if stage == "" {
			stage = GenerationStageAgentGenerate
		}
		return generationFailure(stage, "failed to generate cases: %w", err)
	}

	sections, err := parseGeneratedSections(rawCases)
	if err != nil {
		return generationFailure(GenerationStageParseCases, "failed to parse generated cases: %w", err)
	}
	sections = dedupeGeneratedSections(sections)
	sections = attachCaseContext(sections, task.AffectedProducts, task.AffectedModules)

	if len(sections) == 0 {
		return generationFailure(GenerationStageEmptyCases, "no test cases generated")
	}

	sourceContext := buildSourceContext(
		buildDocumentQueries(requirements, task.AffectedProducts, task.AffectedModules),
		buildKnowledgeQueries(requirements, task.AffectedProducts, task.AffectedModules),
		documentHits,
		retrievedHits,
		knowledgeEntries,
	)

	if err = s.persistGeneratedCases(ctx, taskID, sections, sourceContext); err != nil {
		return err
	}

	return s.updateTaskStatus(ctx, taskID, models.TaskStatusCompleted)
}
