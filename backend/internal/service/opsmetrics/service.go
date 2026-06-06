package opsmetrics

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"caseagent/internal/db/models"

	"github.com/uptrace/bun"
)

type Service struct {
	db bun.IDB
}

type Input struct {
	From         *time.Time
	To           *time.Time
	Provider     string
	Model        string
	WorkflowType string
	TaskID       int
}

type View struct {
	Filters       FilterView           `json:"filters"`
	Summary       Summary              `json:"summary"`
	ByModel       []ModelMetric        `json:"by_model"`
	ByWorkflow    []WorkflowMetric     `json:"by_workflow"`
	FailureStages []FailureStageMetric `json:"failure_stages"`
	JobStatuses   []StatusMetric       `json:"job_statuses"`
}

type FilterView struct {
	From         string `json:"from,omitempty"`
	To           string `json:"to,omitempty"`
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
	WorkflowType string `json:"workflow_type,omitempty"`
	TaskID       int    `json:"task_id,omitempty"`
}

type Summary struct {
	ModelCalls            int     `json:"model_calls"`
	ModelSucceeded        int     `json:"model_succeeded"`
	ModelFailed           int     `json:"model_failed"`
	ModelSuccessRate      float64 `json:"model_success_rate"`
	PromptChars           int     `json:"prompt_chars"`
	ResponseChars         int     `json:"response_chars"`
	TotalChars            int     `json:"total_chars"`
	AccountedTokens       int     `json:"accounted_tokens"`
	Fallbacks             int     `json:"fallbacks"`
	RateLimits            int     `json:"rate_limits"`
	CircuitOpen           int     `json:"circuit_open"`
	BudgetExceeded        int     `json:"budget_exceeded"`
	WorkflowRuns          int     `json:"workflow_runs"`
	WorkflowSucceeded     int     `json:"workflow_succeeded"`
	WorkflowFailed        int     `json:"workflow_failed"`
	WorkflowSuccessRate   float64 `json:"workflow_success_rate"`
	AverageWorkflowMs     int64   `json:"average_workflow_ms"`
	AgentRuns             int     `json:"agent_runs"`
	AgentFailed           int     `json:"agent_failed"`
	BackgroundJobs        int     `json:"background_jobs"`
	BackgroundJobFailed   int     `json:"background_job_failed"`
	BackgroundJobRetrying int     `json:"background_job_retrying"`
	AverageModelLatencyMs int64   `json:"average_model_latency_ms"`
}

type ModelMetric struct {
	Provider              string  `json:"provider"`
	Model                 string  `json:"model"`
	Calls                 int     `json:"calls"`
	Succeeded             int     `json:"succeeded"`
	Failed                int     `json:"failed"`
	SuccessRate           float64 `json:"success_rate"`
	PromptChars           int     `json:"prompt_chars"`
	ResponseChars         int     `json:"response_chars"`
	AccountedTokens       int     `json:"accounted_tokens"`
	Fallbacks             int     `json:"fallbacks"`
	RateLimits            int     `json:"rate_limits"`
	CircuitOpen           int     `json:"circuit_open"`
	BudgetExceeded        int     `json:"budget_exceeded"`
	AverageModelLatencyMs int64   `json:"average_model_latency_ms"`
}

type WorkflowMetric struct {
	WorkflowType      string  `json:"workflow_type"`
	Runs              int     `json:"runs"`
	Succeeded         int     `json:"succeeded"`
	Failed            int     `json:"failed"`
	SuccessRate       float64 `json:"success_rate"`
	AverageDurationMs int64   `json:"average_duration_ms"`
	ModelCalls        int     `json:"model_calls"`
	AccountedTokens   int     `json:"accounted_tokens"`
	FailedAgents      int     `json:"failed_agents"`
	FailedJobs        int     `json:"failed_jobs"`
}

type FailureStageMetric struct {
	Stage     string `json:"stage"`
	Agent     string `json:"agent,omitempty"`
	Failures  int    `json:"failures"`
	LastError string `json:"last_error,omitempty"`
}

type StatusMetric struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type rows struct {
	WorkflowRuns   []models.WorkflowRun
	AgentRuns      []models.AgentRun
	ModelCalls     []models.ModelCall
	BackgroundJobs []models.BackgroundJob
}

func New(db bun.IDB) *Service {
	return &Service{db: db}
}

func (s *Service) Get(ctx context.Context, input Input) (*View, error) {
	if err := validateInput(input); err != nil {
		return nil, err
	}
	loaded, err := s.loadRows(ctx, input)
	if err != nil {
		return nil, err
	}
	filtered := filterRows(input, loaded)
	view := aggregateRows(input, filtered)
	return &view, nil
}

func validateInput(input Input) error {
	if input.TaskID < 0 {
		return fmt.Errorf("task_id must be positive")
	}
	if input.From != nil && input.To != nil && !input.From.Before(*input.To) {
		return fmt.Errorf("from must be before to")
	}
	return nil
}

func (s *Service) loadRows(ctx context.Context, input Input) (rows, error) {
	var out rows

	workflowQuery := s.db.NewSelect().
		Model(&out.WorkflowRuns).
		Order("created_at DESC", "id DESC")
	applyCreatedAtFilter(workflowQuery, input)
	if input.WorkflowType != "" {
		workflowQuery.Where("workflow_type = ?", input.WorkflowType)
	}
	if input.TaskID > 0 {
		workflowQuery.Where("resource_type = ?", "task").Where("resource_id = ?", input.TaskID)
	}
	if err := workflowQuery.Scan(ctx); err != nil {
		return out, err
	}

	agentQuery := s.db.NewSelect().
		Model(&out.AgentRuns).
		Order("created_at DESC", "id DESC")
	applyCreatedAtFilter(agentQuery, input)
	if input.WorkflowType != "" {
		agentQuery.Where("workflow_run_id IN (SELECT id FROM workflow_runs WHERE workflow_type = ?)", input.WorkflowType)
	}
	if input.TaskID > 0 {
		agentQuery.Where("task_id = ?", input.TaskID)
	}
	if err := agentQuery.Scan(ctx); err != nil {
		return out, err
	}

	modelQuery := s.db.NewSelect().
		Model(&out.ModelCalls).
		Order("created_at DESC", "id DESC")
	applyCreatedAtFilter(modelQuery, input)
	if input.Provider != "" {
		modelQuery.Where("provider = ?", input.Provider)
	}
	if input.Model != "" {
		modelQuery.Where("model = ?", input.Model)
	}
	if input.WorkflowType != "" {
		modelQuery.Where("workflow_run_id IN (SELECT id FROM workflow_runs WHERE workflow_type = ?)", input.WorkflowType)
	}
	if input.TaskID > 0 {
		modelQuery.Where(
			"(workflow_run_id IN (SELECT id FROM workflow_runs WHERE resource_type = ? AND resource_id = ?) OR agent_run_id IN (SELECT id FROM agent_runs WHERE task_id = ?))",
			"task",
			input.TaskID,
			input.TaskID,
		)
	}
	if err := modelQuery.Scan(ctx); err != nil {
		return out, err
	}

	jobQuery := s.db.NewSelect().
		Model(&out.BackgroundJobs).
		Order("created_at DESC", "id DESC")
	applyCreatedAtFilter(jobQuery, input)
	if input.WorkflowType != "" {
		jobQuery.Where("job_type = ?", input.WorkflowType)
	}
	if input.TaskID > 0 {
		jobQuery.Where("task_id = ?", input.TaskID)
	}
	if err := jobQuery.Scan(ctx); err != nil {
		return out, err
	}

	return out, nil
}

func applyCreatedAtFilter(query *bun.SelectQuery, input Input) {
	if input.From != nil {
		query.Where("created_at >= ?", *input.From)
	}
	if input.To != nil {
		query.Where("created_at < ?", *input.To)
	}
}

func filterRows(input Input, in rows) rows {
	filtered := rows{
		WorkflowRuns:   filterWorkflows(input, in.WorkflowRuns),
		AgentRuns:      filterAgents(input, in.AgentRuns),
		ModelCalls:     filterModelCalls(input, in.ModelCalls),
		BackgroundJobs: filterJobs(input, in.BackgroundJobs),
	}
	workflowIDsByScope := workflowIDSet(filtered.WorkflowRuns)
	if input.WorkflowType != "" {
		filtered.AgentRuns = keepAgentsInWorkflows(filtered.AgentRuns, workflowIDsByScope)
	}
	agentIDsByScope := agentIDSet(filtered.AgentRuns)
	if input.WorkflowType != "" || input.TaskID > 0 {
		filtered.ModelCalls = keepModelCallsInScope(filtered.ModelCalls, workflowIDsByScope, agentIDsByScope)
	}
	if input.Provider == "" && input.Model == "" {
		return filtered
	}

	workflowIDs := map[int]struct{}{}
	agentIDs := map[int]struct{}{}
	for _, call := range filtered.ModelCalls {
		if call.WorkflowRunID != nil {
			workflowIDs[*call.WorkflowRunID] = struct{}{}
		}
		if call.AgentRunID != nil {
			agentIDs[*call.AgentRunID] = struct{}{}
		}
	}

	agents := make([]models.AgentRun, 0, len(filtered.AgentRuns))
	for _, agent := range filtered.AgentRuns {
		_, agentMatched := agentIDs[agent.ID]
		_, workflowMatched := workflowIDs[pointerValue(agent.WorkflowRunID)]
		if agentMatched || workflowMatched {
			agents = append(agents, agent)
			if agent.WorkflowRunID != nil {
				workflowIDs[*agent.WorkflowRunID] = struct{}{}
			}
		}
	}
	filtered.AgentRuns = agents

	jobIDs := map[int]struct{}{}
	workflows := make([]models.WorkflowRun, 0, len(filtered.WorkflowRuns))
	for _, run := range filtered.WorkflowRuns {
		if _, ok := workflowIDs[run.ID]; ok {
			workflows = append(workflows, run)
			if run.JobID != nil {
				jobIDs[*run.JobID] = struct{}{}
			}
		}
	}
	filtered.WorkflowRuns = workflows

	jobs := make([]models.BackgroundJob, 0, len(filtered.BackgroundJobs))
	for _, job := range filtered.BackgroundJobs {
		_, jobMatched := jobIDs[job.ID]
		_, workflowMatched := workflowIDs[pointerValue(job.WorkflowRunID)]
		if jobMatched || workflowMatched {
			jobs = append(jobs, job)
		}
	}
	filtered.BackgroundJobs = jobs
	return filtered
}

func workflowIDSet(runs []models.WorkflowRun) map[int]struct{} {
	set := make(map[int]struct{}, len(runs))
	for _, run := range runs {
		set[run.ID] = struct{}{}
	}
	return set
}

func agentIDSet(runs []models.AgentRun) map[int]struct{} {
	set := make(map[int]struct{}, len(runs))
	for _, run := range runs {
		set[run.ID] = struct{}{}
	}
	return set
}

func keepAgentsInWorkflows(agents []models.AgentRun, workflowIDs map[int]struct{}) []models.AgentRun {
	out := make([]models.AgentRun, 0, len(agents))
	for _, agent := range agents {
		if _, ok := workflowIDs[pointerValue(agent.WorkflowRunID)]; ok {
			out = append(out, agent)
		}
	}
	return out
}

func keepModelCallsInScope(calls []models.ModelCall, workflowIDs map[int]struct{}, agentIDs map[int]struct{}) []models.ModelCall {
	out := make([]models.ModelCall, 0, len(calls))
	for _, call := range calls {
		_, workflowMatched := workflowIDs[pointerValue(call.WorkflowRunID)]
		_, agentMatched := agentIDs[pointerValue(call.AgentRunID)]
		if workflowMatched || agentMatched {
			out = append(out, call)
		}
	}
	return out
}

func filterWorkflows(input Input, runs []models.WorkflowRun) []models.WorkflowRun {
	out := make([]models.WorkflowRun, 0, len(runs))
	for _, run := range runs {
		if !matchesTime(input, run.CreatedAt) {
			continue
		}
		if input.WorkflowType != "" && run.WorkflowType != input.WorkflowType {
			continue
		}
		if input.TaskID > 0 && (run.ResourceType != "task" || run.ResourceID != input.TaskID) {
			continue
		}
		out = append(out, run)
	}
	return out
}

func filterAgents(input Input, agents []models.AgentRun) []models.AgentRun {
	out := make([]models.AgentRun, 0, len(agents))
	for _, agent := range agents {
		if !matchesTime(input, agent.CreatedAt) {
			continue
		}
		if input.TaskID > 0 && pointerValue(agent.TaskID) != input.TaskID {
			continue
		}
		out = append(out, agent)
	}
	return out
}

func filterModelCalls(input Input, calls []models.ModelCall) []models.ModelCall {
	out := make([]models.ModelCall, 0, len(calls))
	for _, call := range calls {
		if !matchesTime(input, call.CreatedAt) {
			continue
		}
		if input.Provider != "" && call.Provider != input.Provider {
			continue
		}
		if input.Model != "" && call.Model != input.Model {
			continue
		}
		out = append(out, call)
	}
	return out
}

func filterJobs(input Input, jobs []models.BackgroundJob) []models.BackgroundJob {
	out := make([]models.BackgroundJob, 0, len(jobs))
	for _, job := range jobs {
		if !matchesTime(input, job.CreatedAt) {
			continue
		}
		if input.WorkflowType != "" && job.JobType != input.WorkflowType {
			continue
		}
		if input.TaskID > 0 && pointerValue(job.TaskID) != input.TaskID {
			continue
		}
		out = append(out, job)
	}
	return out
}

func aggregateRows(input Input, data rows) View {
	view := View{
		Filters:       filterView(input),
		Summary:       Summary{},
		ByModel:       []ModelMetric{},
		ByWorkflow:    []WorkflowMetric{},
		FailureStages: []FailureStageMetric{},
		JobStatuses:   []StatusMetric{},
	}

	modelMetrics := map[string]*ModelMetric{}
	modelDuration := map[string]durationAccumulator{}
	workflowMetrics := map[string]*WorkflowMetric{}
	workflowDuration := map[string]durationAccumulator{}
	failureStages := map[string]*FailureStageMetric{}
	jobStatuses := map[string]*StatusMetric{}
	var modelDurations durationAccumulator
	var workflowDurations durationAccumulator

	workflowByID := make(map[int]models.WorkflowRun, len(data.WorkflowRuns))
	workflowTypeByID := make(map[int]string, len(data.WorkflowRuns))
	for _, run := range data.WorkflowRuns {
		view.Summary.WorkflowRuns++
		if run.Status == models.WorkflowStatusSucceeded {
			view.Summary.WorkflowSucceeded++
		}
		if run.Status == models.WorkflowStatusFailed {
			view.Summary.WorkflowFailed++
		}
		workflowByID[run.ID] = run
		workflowTypeByID[run.ID] = run.WorkflowType
		key := nonEmpty(run.WorkflowType, "unknown")
		metric := ensureWorkflowMetric(workflowMetrics, key)
		metric.Runs++
		if run.Status == models.WorkflowStatusSucceeded {
			metric.Succeeded++
		}
		if run.Status == models.WorkflowStatusFailed {
			metric.Failed++
		}
		if ms, ok := rowDurationMs(run.StartedAt, run.FinishedAt, run.CreatedAt, run.UpdatedAt); ok {
			workflowDurations.add(ms)
			workflowDuration[key] = workflowDuration[key].with(ms)
		}
	}

	for _, agent := range data.AgentRuns {
		view.Summary.AgentRuns++
		if agent.WorkflowRunID != nil {
			if runType := workflowTypeByID[*agent.WorkflowRunID]; runType != "" {
				if metric := ensureWorkflowMetric(workflowMetrics, runType); agent.Status == models.WorkflowStatusFailed {
					metric.FailedAgents++
				}
			}
		}
		if agent.Status != models.WorkflowStatusFailed {
			continue
		}
		view.Summary.AgentFailed++
		stageKey := nonEmpty(agent.Stage, "unknown")
		if agent.AgentName != "" {
			stageKey = agent.AgentName + ":" + stageKey
		}
		metric := failureStages[stageKey]
		if metric == nil {
			metric = &FailureStageMetric{Stage: nonEmpty(agent.Stage, "unknown"), Agent: agent.AgentName}
			failureStages[stageKey] = metric
		}
		metric.Failures++
		if agent.LastError != "" {
			metric.LastError = agent.LastError
		}
	}

	for _, call := range data.ModelCalls {
		view.Summary.ModelCalls++
		view.Summary.PromptChars += call.PromptChars
		view.Summary.ResponseChars += call.ResponseChars
		view.Summary.TotalChars += call.PromptChars + call.ResponseChars
		tokens := accountedTokens(call)
		view.Summary.AccountedTokens += tokens
		if call.Status == models.WorkflowStatusSucceeded {
			view.Summary.ModelSucceeded++
		}
		if call.Status == models.WorkflowStatusFailed {
			view.Summary.ModelFailed++
		}
		signals := callSignals(call)
		if signals.fallback {
			view.Summary.Fallbacks++
		}
		if signals.rateLimit {
			view.Summary.RateLimits++
		}
		if signals.circuitOpen {
			view.Summary.CircuitOpen++
		}
		if signals.budgetExceeded {
			view.Summary.BudgetExceeded++
		}

		modelKey := nonEmpty(call.Provider, "unknown") + "\x00" + nonEmpty(call.Model, "unknown")
		modelMetric := modelMetrics[modelKey]
		if modelMetric == nil {
			modelMetric = &ModelMetric{Provider: nonEmpty(call.Provider, "unknown"), Model: nonEmpty(call.Model, "unknown")}
			modelMetrics[modelKey] = modelMetric
		}
		addModelCall(modelMetric, call, tokens, signals)
		if ms, ok := modelDurationMs(call); ok {
			modelDurations.add(ms)
			modelDuration[modelKey] = modelDuration[modelKey].with(ms)
		}

		runType := workflowTypeForCall(call, workflowByID, workflowTypeByID)
		if runType != "" {
			workflowMetric := ensureWorkflowMetric(workflowMetrics, runType)
			workflowMetric.ModelCalls++
			workflowMetric.AccountedTokens += tokens
		}
	}

	for _, job := range data.BackgroundJobs {
		view.Summary.BackgroundJobs++
		status := jobStatusForMetrics(job)
		if status == models.JobStatusFailed {
			view.Summary.BackgroundJobFailed++
		}
		if status == "retrying" {
			view.Summary.BackgroundJobRetrying++
		}
		key := job.JobType + "\x00" + status
		metric := jobStatuses[key]
		if metric == nil {
			metric = &StatusMetric{Type: nonEmpty(job.JobType, "unknown"), Status: status}
			jobStatuses[key] = metric
		}
		metric.Count++

		if job.Status == models.JobStatusFailed {
			workflowMetric := ensureWorkflowMetric(workflowMetrics, nonEmpty(job.JobType, "unknown"))
			workflowMetric.FailedJobs++
		}
	}

	view.Summary.ModelSuccessRate = ratio(view.Summary.ModelSucceeded, view.Summary.ModelCalls)
	view.Summary.WorkflowSuccessRate = ratio(view.Summary.WorkflowSucceeded, view.Summary.WorkflowRuns)
	view.Summary.AverageWorkflowMs = workflowDurations.average()
	view.Summary.AverageModelLatencyMs = modelDurations.average()
	view.ByModel = sortedModelMetrics(modelMetrics, modelDuration)
	view.ByWorkflow = sortedWorkflowMetrics(workflowMetrics, workflowDuration)
	view.FailureStages = sortedFailureStages(failureStages)
	view.JobStatuses = sortedStatusMetrics(jobStatuses)
	return view
}

func addModelCall(metric *ModelMetric, call models.ModelCall, tokens int, signals callSignalSet) {
	metric.Calls++
	metric.PromptChars += call.PromptChars
	metric.ResponseChars += call.ResponseChars
	metric.AccountedTokens += tokens
	if call.Status == models.WorkflowStatusSucceeded {
		metric.Succeeded++
	}
	if call.Status == models.WorkflowStatusFailed {
		metric.Failed++
	}
	if signals.fallback {
		metric.Fallbacks++
	}
	if signals.rateLimit {
		metric.RateLimits++
	}
	if signals.circuitOpen {
		metric.CircuitOpen++
	}
	if signals.budgetExceeded {
		metric.BudgetExceeded++
	}
	metric.SuccessRate = ratio(metric.Succeeded, metric.Calls)
}

func ensureWorkflowMetric(metrics map[string]*WorkflowMetric, workflowType string) *WorkflowMetric {
	key := nonEmpty(workflowType, "unknown")
	metric := metrics[key]
	if metric == nil {
		metric = &WorkflowMetric{WorkflowType: key}
		metrics[key] = metric
	}
	return metric
}

func sortedModelMetrics(metrics map[string]*ModelMetric, durations map[string]durationAccumulator) []ModelMetric {
	keys := make([]string, 0, len(metrics))
	for key := range metrics {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left := metrics[keys[i]]
		right := metrics[keys[j]]
		if left.AccountedTokens == right.AccountedTokens {
			return left.Provider+"/"+left.Model < right.Provider+"/"+right.Model
		}
		return left.AccountedTokens > right.AccountedTokens
	})
	rows := make([]ModelMetric, 0, len(keys))
	for _, key := range keys {
		row := *metrics[key]
		row.AverageModelLatencyMs = durations[key].average()
		rows = append(rows, row)
	}
	return rows
}

func sortedWorkflowMetrics(metrics map[string]*WorkflowMetric, durations map[string]durationAccumulator) []WorkflowMetric {
	keys := make([]string, 0, len(metrics))
	for key := range metrics {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rows := make([]WorkflowMetric, 0, len(keys))
	for _, key := range keys {
		row := *metrics[key]
		row.SuccessRate = ratio(row.Succeeded, row.Runs)
		row.AverageDurationMs = durations[key].average()
		rows = append(rows, row)
	}
	return rows
}

func sortedFailureStages(metrics map[string]*FailureStageMetric) []FailureStageMetric {
	keys := make([]string, 0, len(metrics))
	for key := range metrics {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if metrics[keys[i]].Failures == metrics[keys[j]].Failures {
			return keys[i] < keys[j]
		}
		return metrics[keys[i]].Failures > metrics[keys[j]].Failures
	})
	rows := make([]FailureStageMetric, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, *metrics[key])
	}
	return rows
}

func sortedStatusMetrics(metrics map[string]*StatusMetric) []StatusMetric {
	keys := make([]string, 0, len(metrics))
	for key := range metrics {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rows := make([]StatusMetric, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, *metrics[key])
	}
	return rows
}

func workflowTypeForCall(call models.ModelCall, workflowByID map[int]models.WorkflowRun, workflowTypeByID map[int]string) string {
	if call.WorkflowRunID == nil {
		return ""
	}
	if runType := workflowTypeByID[*call.WorkflowRunID]; runType != "" {
		return runType
	}
	if run, ok := workflowByID[*call.WorkflowRunID]; ok {
		return run.WorkflowType
	}
	return ""
}

func filterView(input Input) FilterView {
	view := FilterView{
		Provider:     input.Provider,
		Model:        input.Model,
		WorkflowType: input.WorkflowType,
		TaskID:       input.TaskID,
	}
	if input.From != nil {
		view.From = input.From.Format(time.RFC3339)
	}
	if input.To != nil {
		view.To = input.To.Format(time.RFC3339)
	}
	return view
}

type callSignalSet struct {
	fallback       bool
	rateLimit      bool
	circuitOpen    bool
	budgetExceeded bool
}

func callSignals(call models.ModelCall) callSignalSet {
	metadata := call.Metadata
	guardrail := stringFromAny(metadata["guardrail_event"])
	errText := strings.ToLower(call.LastError)
	return callSignalSet{
		fallback:       stringFromAny(metadata["provider_role"]) == "fallback",
		rateLimit:      strings.Contains(errText, "rate limit") || strings.Contains(errText, "429"),
		circuitOpen:    guardrail == "circuit_open",
		budgetExceeded: guardrail == "budget_exceeded",
	}
}

func accountedTokens(call models.ModelCall) int {
	cost := mapFromAny(call.Metadata["cost"])
	for _, key := range []string{"accounted_tokens", "total_tokens", "estimated_total_tokens"} {
		if value := intFromAny(cost[key]); value > 0 {
			return value
		}
	}
	return estimateTokens(call.PromptChars + call.ResponseChars)
}

func modelDurationMs(call models.ModelCall) (int64, bool) {
	if value := intFromAny(call.Metadata["elapsed_ms"]); value > 0 {
		return int64(value), true
	}
	return rowDurationMs(call.StartedAt, call.FinishedAt, call.CreatedAt, call.UpdatedAt)
}

func rowDurationMs(started *time.Time, finished *time.Time, created time.Time, updated time.Time) (int64, bool) {
	start := created
	if started != nil {
		start = *started
	}
	finish := updated
	if finished != nil {
		finish = *finished
	}
	if finish.Before(start) || finish.Equal(start) {
		return 0, false
	}
	return finish.Sub(start).Milliseconds(), true
}

type durationAccumulator struct {
	total int64
	count int64
}

func (a durationAccumulator) with(ms int64) durationAccumulator {
	a.add(ms)
	return a
}

func (a *durationAccumulator) add(ms int64) {
	if ms <= 0 {
		return
	}
	a.total += ms
	a.count++
}

func (a durationAccumulator) average() int64 {
	if a.count == 0 {
		return 0
	}
	return a.total / a.count
}

func jobStatusForMetrics(job models.BackgroundJob) string {
	if job.Status == models.JobStatusPending && job.RetryCount > 0 {
		return "retrying"
	}
	return nonEmpty(job.Status, "unknown")
}

func matchesTime(input Input, value time.Time) bool {
	if input.From != nil && value.Before(*input.From) {
		return false
	}
	if input.To != nil && !value.Before(*input.To) {
		return false
	}
	return true
}

func ratio(part int, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total)
}

func estimateTokens(chars int) int {
	if chars <= 0 {
		return 0
	}
	return (chars + 3) / 4
}

func nonEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func pointerValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func mapFromAny(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	default:
		return nil
	}
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	default:
		return 0
	}
}

func stringFromAny(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}
