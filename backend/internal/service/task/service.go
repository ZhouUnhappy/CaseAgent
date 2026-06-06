package task

import (
	"context"
	"encoding/json"
	"log/slog"

	"caseagent/internal/db/models"
	"caseagent/internal/generation"
	agentservice "caseagent/internal/service/agent"
	retrievalservice "caseagent/internal/service/retrieval"
	workflowservice "caseagent/internal/service/workflow"

	"github.com/uptrace/bun"
)

type Service struct {
	db                    bun.IDB
	traceRecorder         *workflowservice.Recorder
	newCaseGenerator      func(ctx context.Context) (CaseGenerator, error)
	newRetriever          func() Retriever
	newSuggestionRecorder func() SuggestionRecorder
}

func New(db bun.IDB) *Service {
	return &Service{db: db}
}

func (s *Service) WithTraceRecorder(recorder *workflowservice.Recorder) *Service {
	s.traceRecorder = recorder
	return s
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
	ctx = workflowservice.WithTaskID(ctx, taskID)
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
	profile := generation.CurrentProfile()
	runID := workflowservice.RunIDPointerFromContext(ctx)
	recordGenerationProfileTrace(ctx, s.db, runID, profile)

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
	recordRetrievalTrace(ctx, s.workflowRecorder(), taskID, runID,
		buildDocumentQueries(requirements, task.AffectedProducts, task.AffectedModules),
		documentHits,
		buildKnowledgeQueries(requirements, task.AffectedProducts, task.AffectedModules),
		retrievedHits,
	)

	agentSvc, err := s.caseGenerator(ctx)
	if err != nil {
		return generationFailure(GenerationStageInitializeAgent, "failed to initialize agent service: %w", err)
	}

	knowledgeContext := formatKnowledgeContext(knowledgeEntries)
	modelCallCollector := agentservice.NewModelCallCollector()
	ctx = agentservice.WithModelCallCollector(ctx, modelCallCollector)
	rawCases, err := agentSvc.GenerateCases(ctx, requirementsContext, knowledgeContext)
	recordAgentTrace(ctx, s.workflowRecorder(), taskID, runID, requirementsContext, knowledgeContext, rawCases, err)
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
		modelCallCollector.Calls(),
	)
	attachGenerationProfile(sourceContext, profile)

	if err = s.persistGeneratedCases(ctx, taskID, sections, sourceContext); err != nil {
		return err
	}
	recordGeneratedCasesArtifact(ctx, s.workflowRecorder(), taskID, runID, sections, sourceContext)

	return s.updateTaskStatus(ctx, taskID, models.TaskStatusCompleted)
}

func recordRetrievalTrace(
	ctx context.Context,
	recorder *workflowservice.Recorder,
	taskID int,
	runID *int,
	documentQueries []string,
	documentHits []retrievalservice.DocumentResult,
	knowledgeQueries []string,
	knowledgeHits []retrievalservice.KnowledgeResult,
) {
	taskIDPtr := &taskID
	if _, err := recorder.RecordRetrievalRun(ctx, workflowservice.RetrievalRunInput{
		WorkflowRunID: runID,
		TaskID:        taskIDPtr,
		RetrieverType: "documents",
		QueryCount:    len(documentQueries),
		HitCount:      len(documentHits),
		Metadata: map[string]any{
			"queries": documentQueries,
		},
	}); err != nil {
		slog.Warn("document retrieval trace record failed", "task_id", taskID, "error", err)
	}
	if _, err := recorder.RecordRetrievalRun(ctx, workflowservice.RetrievalRunInput{
		WorkflowRunID: runID,
		TaskID:        taskIDPtr,
		RetrieverType: "knowledge",
		QueryCount:    len(knowledgeQueries),
		HitCount:      len(knowledgeHits),
		Metadata: map[string]any{
			"queries": knowledgeQueries,
		},
	}); err != nil {
		slog.Warn("knowledge retrieval trace record failed", "task_id", taskID, "error", err)
	}
}

func recordAgentTrace(ctx context.Context, recorder *workflowservice.Recorder, taskID int, runID *int, requirements string, knowledge string, output string, cause error) {
	status := models.WorkflowStatusSucceeded
	lastErr := ""
	if cause != nil {
		status = models.WorkflowStatusFailed
		lastErr = cause.Error()
	}
	taskIDPtr := &taskID
	if _, err := recorder.RecordAgentRun(ctx, workflowservice.AgentRunInput{
		WorkflowRunID: runID,
		TaskID:        taskIDPtr,
		AgentName:     "case_generation",
		Stage:         "generate_cases",
		Status:        status,
		InputSummary:  summarizeTraceText(requirements) + "\n\n" + summarizeTraceText(knowledge),
		OutputSummary: summarizeTraceText(output),
		LastError:     lastErr,
		Metadata: map[string]any{
			"requirements_chars": len(requirements),
			"knowledge_chars":    len(knowledge),
			"output_chars":       len(output),
		},
	}); err != nil {
		slog.Warn("agent trace record failed", "task_id", taskID, "error", err)
	}
}

func recordGenerationProfileTrace(ctx context.Context, db bun.IDB, runID *int, profile generation.Profile) {
	if db == nil || runID == nil || *runID <= 0 {
		return
	}
	run := new(models.WorkflowRun)
	if err := db.NewSelect().
		Model(run).
		Column("id", "metadata").
		Where("id = ?", *runID).
		Scan(ctx); err != nil {
		slog.Warn("generation profile workflow trace load failed", "workflow_run_id", *runID, "error", err)
		return
	}
	metadata := map[string]any{}
	for key, value := range run.Metadata {
		metadata[key] = value
	}
	metadata["generation_profile"] = profile
	metadata["generation_profile_id"] = profile.ID
	metadata["generation_profile_version"] = profile.Version
	if _, err := db.NewUpdate().
		Model((*models.WorkflowRun)(nil)).
		Set("metadata = ?", metadata).
		Where("id = ?", *runID).
		Exec(ctx); err != nil {
		slog.Warn("generation profile workflow trace update failed", "workflow_run_id", *runID, "error", err)
	}
}

func attachGenerationProfile(sourceContext map[string]any, profile generation.Profile) {
	if sourceContext == nil {
		return
	}
	sourceContext["generation_profile"] = profile
	sourceContext["generation_profile_id"] = profile.ID
	sourceContext["generation_profile_version"] = profile.Version
}

func recordGeneratedCasesArtifact(ctx context.Context, recorder *workflowservice.Recorder, taskID int, runID *int, sections []generatedSection, sourceContext map[string]any) {
	payload, err := json.Marshal(sections)
	if err != nil {
		slog.Warn("generated cases artifact marshal failed", "task_id", taskID, "error", err)
		return
	}
	taskIDPtr := &taskID
	if _, err := recorder.RecordArtifact(ctx, workflowservice.ArtifactInput{
		WorkflowRunID: runID,
		ArtifactType:  models.ArtifactTypeGeneratedCases,
		ResourceType:  "task",
		ResourceID:    taskIDPtr,
		Name:          "deduped generated cases",
		Content:       string(payload),
		Payload: map[string]any{
			"section_count":  len(sections),
			"case_count":     countGeneratedCases(sections),
			"source_context": sourceContext,
		},
	}); err != nil {
		slog.Warn("generated cases artifact record failed", "task_id", taskID, "error", err)
	}
}

func countGeneratedCases(sections []generatedSection) int {
	total := 0
	for _, section := range sections {
		total += len(section.Cases)
	}
	return total
}

func summarizeTraceText(value string) string {
	const max = 800
	if len(value) <= max {
		return value
	}
	return value[:max]
}
