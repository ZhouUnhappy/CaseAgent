// Package agent owns the orchestration boundary between the application's
// generation pipeline and the eino agent layer.
//
// Responsibility split:
//
//   - Service (this package) is the *application-level coordinator*. It owns
//     the ADK/AgentGraph nodes for functional / ops / failure / boundary,
//     applies retry-once on transient failures, isolates individual node
//     failures so they don't block sibling agents, dedupes the merged
//     sections, and finally hands the consolidated draft to DeepAgent for
//     refinement. Service does not call the LLM directly.
//
//   - DeepAgent (backend/internal/agent/deep) is the *agent-level coordinator*
//     that talks to the chat model. It serves two roles: (a) fallback —
//     when every sub-agent fails or produces no parseable output, DeepAgent
//     generates a full set of sections from scratch; (b) refinement —
//     consolidating the dedup'd sub-agent draft into a single coherent
//     output before persistence.
//
// Sub-agents are exposed through eino ADK Agent adapters and consumed by both
// the application AgentGraph and DeepAgent's fallback coordination path.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"caseagent/internal/agent/boundary"
	"caseagent/internal/agent/deep"
	"caseagent/internal/agent/failure"
	"caseagent/internal/agent/functional"
	"caseagent/internal/agent/ops"
	"caseagent/internal/agent/prompts"
	"caseagent/internal/ai"
	"caseagent/internal/config"
	"caseagent/internal/db/models"
	workflowservice "caseagent/internal/service/workflow"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/uptrace/bun"
)

type Service struct {
	deepAgent       *deep.Agent
	functionalAgent *functional.Agent
	opsAgent        *ops.Agent
	failureAgent    *failure.Agent
	boundaryAgent   *boundary.Agent
	subAgents       []adk.Agent
	chatCallTimeout time.Duration
	traceRecorder   workflowservice.AgentTraceRecorder
}

type Config struct {
	ChatModel       model.BaseChatModel // Optional, if not provided will initialize from config
	ChatCallTimeout time.Duration       // Optional, defaults to model.chat.request_timeout_seconds
	TraceDB         bun.IDB             // Optional, records workflow model_calls when a workflow run is in context
	TraceRecorder   workflowservice.AgentTraceRecorder
	ChatProvider    string // Optional, used for model_call trace rows when ChatModel is provided
	ChatModelName   string // Optional, used for model_call trace rows when ChatModel is provided
	Guardrails      GuardrailConfig
	PromptRegistry  *prompts.Registry
}

const GenerationStageDeepAgentFallback = "deep_agent_fallback"

const defaultChatCallTimeout = 60 * time.Second

type GenerationError struct {
	Stage string
	Err   error
}

func (e *GenerationError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return fmt.Sprintf("%s: %v", e.Stage, e.Err)
}

func (e *GenerationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func FailureStage(err error) string {
	var generationErr *GenerationError
	if errors.As(err, &generationErr) {
		return strings.TrimSpace(generationErr.Stage)
	}
	return ""
}

func generationError(stage string, err error) error {
	if err == nil {
		return nil
	}
	return &GenerationError{Stage: stage, Err: err}
}

func New(ctx context.Context, cfg *Config) (*Service, error) {
	if cfg == nil {
		cfg = &Config{}
	}

	var chatModel model.BaseChatModel
	var err error
	callTimeout := cfg.ChatCallTimeout
	provider := strings.TrimSpace(cfg.ChatProvider)
	modelName := strings.TrimSpace(cfg.ChatModelName)
	guardrails := cfg.Guardrails

	// If ChatModel is provided, use it; otherwise initialize from config
	if cfg.ChatModel != nil {
		chatModel = cfg.ChatModel
		if provider == "" {
			provider = "custom"
		}
	} else {
		// Initialize chat model from config
		appCfg := config.Get()
		chatModel, err = ai.NewChatModel(ctx, appCfg.Model.Chat)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize chat model: %w", err)
		}
		if provider == "" {
			provider = appCfg.Model.Chat.Provider
		}
		if modelName == "" {
			modelName = appCfg.Model.Chat.Model
		}
		guardrails, err = configuredGuardrails(ctx, appCfg.Model.Chat, guardrails)
		if err != nil {
			return nil, err
		}
	}
	if callTimeout == 0 {
		callTimeout = configuredChatCallTimeout(config.Get())
	}
	traceRecorder := cfg.TraceRecorder
	if traceRecorder == nil && cfg.TraceDB != nil {
		traceRecorder = workflowservice.NewRecorder(cfg.TraceDB)
	}
	fallbackModel := guardrails.FallbackChatModel
	if fallbackModel != nil {
		fallbackModel = traceChatModel(fallbackModel, traceRecorder, guardrails.FallbackProvider, guardrails.FallbackModelName, "fallback")
		guardrails.FallbackChatModel = fallbackModel
	}
	chatModel = traceChatModel(chatModel, traceRecorder, provider, modelName, "primary")
	chatModel = guardChatModel(chatModel, traceRecorder, provider, modelName, guardrails)
	promptRegistry := cfg.PromptRegistry
	if promptRegistry == nil {
		promptRegistry = prompts.DefaultRegistry()
	}

	// Create sub-agents
	functionalAgent, err := functional.New(ctx, &functional.Config{ChatModel: chatModel, Prompts: promptRegistry})
	if err != nil {
		return nil, fmt.Errorf("failed to create functional agent: %w", err)
	}

	opsAgent, err := ops.New(ctx, &ops.Config{ChatModel: chatModel, Prompts: promptRegistry})
	if err != nil {
		return nil, fmt.Errorf("failed to create ops agent: %w", err)
	}

	failureAgent, err := failure.New(ctx, &failure.Config{ChatModel: chatModel, Prompts: promptRegistry})
	if err != nil {
		return nil, fmt.Errorf("failed to create failure agent: %w", err)
	}

	boundaryAgent, err := boundary.New(ctx, &boundary.Config{ChatModel: chatModel, Prompts: promptRegistry})
	if err != nil {
		return nil, fmt.Errorf("failed to create boundary agent: %w", err)
	}

	subAgents := []adk.Agent{
		newCaseGenerationADKAgent("functional", "Generate functional test cases", functionalAgent.GenerateFunctionalCases),
		newCaseGenerationADKAgent("ops", "Generate operations test cases", opsAgent.GenerateOpsCases),
		newCaseGenerationADKAgent("failure", "Generate failure-mode test cases", failureAgent.GenerateFailureCases),
		newCaseGenerationADKAgent("boundary", "Generate boundary test cases", boundaryAgent.GenerateBoundaryCases),
	}

	deepAgent, err := deep.New(ctx, &deep.Config{
		ChatModel: chatModel,
		SubAgents: subAgents,
		Prompts:   promptRegistry,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create deep agent: %w", err)
	}

	return &Service{
		deepAgent:       deepAgent,
		functionalAgent: functionalAgent,
		opsAgent:        opsAgent,
		failureAgent:    failureAgent,
		boundaryAgent:   boundaryAgent,
		subAgents:       subAgents,
		chatCallTimeout: callTimeout,
		traceRecorder:   traceRecorder,
	}, nil
}

func configuredChatCallTimeout(appCfg *config.Config) time.Duration {
	if appCfg == nil || appCfg.Model.Chat.RequestTimeoutSeconds <= 0 {
		return defaultChatCallTimeout
	}
	return time.Duration(appCfg.Model.Chat.RequestTimeoutSeconds) * time.Second
}

func configuredGuardrails(ctx context.Context, chatCfg config.ChatModelConfig, overrides GuardrailConfig) (GuardrailConfig, error) {
	guardrails := overrides
	if guardrails.TaskBudgetTokens == 0 {
		guardrails.TaskBudgetTokens = chatCfg.TaskBudgetTokens
	}
	if guardrails.ProviderTimeout == 0 && chatCfg.ProviderTimeoutSeconds > 0 {
		guardrails.ProviderTimeout = time.Duration(chatCfg.ProviderTimeoutSeconds) * time.Second
	}
	if guardrails.FailureThreshold == 0 {
		guardrails.FailureThreshold = chatCfg.CircuitBreakerFailureThreshold
	}
	if guardrails.CircuitOpenCooldown == 0 && chatCfg.CircuitBreakerCooldownSeconds > 0 {
		guardrails.CircuitOpenCooldown = time.Duration(chatCfg.CircuitBreakerCooldownSeconds) * time.Second
	}
	if guardrails.FallbackChatModel != nil || strings.TrimSpace(chatCfg.Fallback.Provider) == "" {
		return guardrails, nil
	}

	fallbackCfg := chatFallbackToChatConfig(chatCfg.Fallback)
	fallbackModel, err := ai.NewChatModel(ctx, fallbackCfg)
	if err != nil {
		return guardrails, fmt.Errorf("failed to initialize fallback chat model: %w", err)
	}
	guardrails.FallbackChatModel = fallbackModel
	guardrails.FallbackProvider = fallbackCfg.Provider
	guardrails.FallbackModelName = fallbackCfg.Model
	if guardrails.FallbackProviderTimeout == 0 && chatCfg.Fallback.ProviderTimeoutSeconds > 0 {
		guardrails.FallbackProviderTimeout = time.Duration(chatCfg.Fallback.ProviderTimeoutSeconds) * time.Second
	}
	return guardrails, nil
}

func chatFallbackToChatConfig(cfg config.ChatFallbackConfig) config.ChatModelConfig {
	return config.ChatModelConfig{
		Provider: cfg.Provider,
		Model:    cfg.Model,
		APIKey:   cfg.APIKey,
		BaseURL:  cfg.BaseURL,
		Region:   cfg.Region,
	}
}

// GenerateCases runs each sub-agent (with retry-once on transient failure),
// dedupes the merged sections, and asks DeepAgent to refine the consolidated
// draft. A single sub-agent failing — even after its retry — is logged and
// skipped; sibling sub-agents continue running and their outputs still flow
// through dedupe → refinement → persistence.
func (s *Service) GenerateCases(ctx context.Context, requirements string, knowledge string) (string, error) {
	graph := newAgentGraph([]agentGraphNode{
		{Name: "functional", Agent: s.subAgents[0]},
		{Name: "ops", Agent: s.subAgents[1]},
		{Name: "failure", Agent: s.subAgents[2]},
		{Name: "boundary", Agent: s.subAgents[3]},
	}, s.chatCallTimeout, s.traceRecorder)

	graphResult := graph.Run(ctx, requirements, knowledge)
	if len(graphResult.Sections) == 0 {
		slog.Warn("all sub-agents produced no usable output; falling back to DeepAgent.GenerateCases",
			"sub_agent_count", len(graphResult.Nodes))
		return s.generateFallback(ctx, requirements, knowledge, "no_usable_sub_agent_output")
	}

	normalized := dedupeGeneratedSections(graphResult.Sections)
	if len(normalized) == 0 {
		slog.Warn("dedupe collapsed all sub-agent sections; falling back to DeepAgent.GenerateCases")
		return s.generateFallback(ctx, requirements, knowledge, "dedupe_empty")
	}

	payload, err := json.Marshal(normalized)
	if err != nil {
		slog.Warn("failed to marshal dedup'd sections; falling back to DeepAgent.GenerateCases", "error", err)
		return s.generateFallback(ctx, requirements, knowledge, "marshal_deduped_sections_failed")
	}

	refined, err := runTimedAgentCall(ctx, "deep_refine", "initial", s.chatCallTimeout, s.traceRecorder, func(ctx context.Context) (string, error) {
		return s.deepAgent.RefineCases(ctx, requirements, knowledge, string(payload))
	}, withAgentCallMetadata(map[string]any{
		"graph_node_type": "coordinator",
		"trigger_reason":  "sub_agent_sections_ready",
		"section_count":   len(normalized),
		"input_chars":     len(requirements) + len(knowledge) + len(payload),
		"draft_chars":     len(payload),
	}))
	if err != nil || strings.TrimSpace(refined) == "" {
		if err != nil {
			slog.Warn("DeepAgent.RefineCases failed; returning unrefined dedup'd payload", "error", err)
		} else {
			slog.Warn("DeepAgent.RefineCases returned empty content; returning unrefined dedup'd payload")
		}
		return string(payload), nil
	}
	if _, parseErr := parseGeneratedSections(refined); parseErr != nil {
		slog.Warn("DeepAgent.RefineCases produced unparseable output; returning unrefined dedup'd payload",
			"parse_err", parseErr)
		return string(payload), nil
	}
	return refined, nil
}

func (s *Service) generateFallback(ctx context.Context, requirements string, knowledge string, reason string) (string, error) {
	result, err := runTimedAgentCall(ctx, "deep_fallback", "initial", s.chatCallTimeout, s.traceRecorder, func(ctx context.Context) (string, error) {
		return s.deepAgent.GenerateCases(ctx, requirements, knowledge)
	}, withAgentCallMetadata(map[string]any{
		"graph_node_type": "coordinator",
		"trigger_reason":  reason,
		"input_chars":     len(requirements) + len(knowledge),
	}))
	if err != nil {
		return "", generationError(GenerationStageDeepAgentFallback, err)
	}
	return result, nil
}

// runSubAgentWithRetry invokes fn once and, on error, retries exactly one
// more time. It logs the first-attempt error so that intermittent provider
// failures (rate limits, transient 5xx) are visible in operator logs.
func runSubAgentWithRetry(ctx context.Context, name string, timeout time.Duration, recorder workflowservice.AgentTraceRecorder, fn func(context.Context) (string, error), opts ...agentCallOption) (string, error) {
	output, err := runTimedAgentCall(ctx, name, "initial", timeout, recorder, fn, opts...)
	if err == nil {
		return output, nil
	}
	if isContextError(err) {
		return "", err
	}
	slog.Warn("sub-agent first attempt failed; retrying once", "agent", name, "error", err)
	return runTimedAgentCall(ctx, name, "retry", timeout, recorder, fn, opts...)
}

type agentCallOptions struct {
	metadata     map[string]any
	inputSummary string
}

type agentCallOption func(*agentCallOptions)

func withAgentCallMetadata(metadata map[string]any) agentCallOption {
	return func(opts *agentCallOptions) {
		opts.metadata = mergeMetadata(opts.metadata, metadata)
	}
}

func withAgentCallInputSummary(summary string) agentCallOption {
	return func(opts *agentCallOptions) {
		opts.inputSummary = summary
	}
}

func runTimedAgentCall(
	ctx context.Context,
	name string,
	attempt string,
	timeout time.Duration,
	recorder workflowservice.AgentTraceRecorder,
	fn func(context.Context) (string, error),
	opts ...agentCallOption,
) (string, error) {
	callOpts := agentCallOptions{metadata: map[string]any{}}
	for _, opt := range opts {
		if opt != nil {
			opt(&callOpts)
		}
	}
	callOpts.metadata = mergeMetadata(callOpts.metadata, map[string]any{
		"attempt": attempt,
	})

	callCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	started := time.Now()
	slog.Info("agent call started",
		"agent", name,
		"attempt", attempt,
		"timeout", timeout.String())

	agentRunID := startTracedAgentRun(ctx, recorder, name, attempt, callOpts.inputSummary, callOpts.metadata)
	tracedCtx := withModelTrace(callCtx, name, attempt)
	if agentRunID > 0 {
		tracedCtx = workflowservice.WithAgentRunID(tracedCtx, agentRunID)
	}

	output, err := fn(tracedCtx)
	elapsed := time.Since(started)
	if err == nil {
		finishTracedAgentRun(ctx, recorder, agentRunID, models.WorkflowStatusSucceeded, output, nil, elapsed, callOpts.metadata)
		slog.Info("agent call completed",
			"agent", name,
			"attempt", attempt,
			"elapsed_ms", elapsed.Milliseconds(),
			"output_bytes", len(output))
		return output, nil
	}
	if callCtx.Err() != nil {
		err = callCtx.Err()
	}
	finishTracedAgentRun(ctx, recorder, agentRunID, models.WorkflowStatusFailed, "", err, elapsed, callOpts.metadata)
	slog.Warn("agent call failed",
		"agent", name,
		"attempt", attempt,
		"elapsed_ms", elapsed.Milliseconds(),
		"error", err)
	return "", err
}

func startTracedAgentRun(ctx context.Context, recorder workflowservice.AgentTraceRecorder, name string, attempt string, inputSummary string, metadata map[string]any) int {
	if recorder == nil {
		return 0
	}
	row, err := recorder.StartAgentRun(ctx, workflowservice.StartAgentRunInput{
		WorkflowRunID: workflowservice.RunIDPointerFromContext(ctx),
		TaskID:        workflowservice.TaskIDPointerFromContext(ctx),
		AgentName:     name,
		Stage:         attempt,
		InputSummary:  inputSummary,
		Metadata:      defaultMetadata(metadata),
	})
	if err != nil {
		slog.Warn("agent run trace start failed", "agent", name, "attempt", attempt, "error", err)
		return 0
	}
	return row.ID
}

func finishTracedAgentRun(ctx context.Context, recorder workflowservice.AgentTraceRecorder, agentRunID int, status string, output string, cause error, elapsed time.Duration, metadata map[string]any) {
	if recorder == nil || agentRunID <= 0 {
		return
	}
	lastErr := ""
	if cause != nil {
		lastErr = cause.Error()
	}
	if err := recorder.FinishAgentRun(ctx, agentRunID, workflowservice.FinishAgentRunInput{
		Status:        status,
		OutputSummary: output,
		LastError:     lastErr,
		Metadata: mergeMetadata(metadata, map[string]any{
			"elapsed_ms": elapsed.Milliseconds(),
		}),
	}); err != nil {
		slog.Warn("agent run trace finish failed", "agent_run_id", agentRunID, "status", status, "error", err)
	}
}

func defaultMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}
	return mergeMetadata(metadata, nil)
}

func mergeMetadata(base map[string]any, extra map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(extra))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range extra {
		merged[key] = value
	}
	return merged
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

type generatedSection struct {
	Section string           `json:"section"`
	Cases   []map[string]any `json:"cases"`
}

func extractJSONArrayPayload(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
			lines = lines[1:]
		}
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
			lines = lines[:len(lines)-1]
		}
		trimmed = strings.Join(lines, "\n")
	}

	trimmed = strings.TrimSpace(trimmed)
	start := strings.Index(trimmed, "[")
	end := strings.LastIndex(trimmed, "]")
	if start < 0 || end <= start {
		return ""
	}

	payload := strings.TrimSpace(trimmed[start+1 : end])
	return payload
}

func parseGeneratedSections(raw string) ([]generatedSection, error) {
	cleaned := extractJSONPayload(raw)
	if cleaned == "" {
		return nil, fmt.Errorf("empty model response")
	}

	var sections []generatedSection
	if err := json.Unmarshal([]byte(cleaned), &sections); err == nil && len(sections) > 0 {
		return sections, nil
	}

	var single generatedSection
	if err := json.Unmarshal([]byte(cleaned), &single); err == nil && (single.Section != "" || len(single.Cases) > 0) {
		return []generatedSection{single}, nil
	}

	return nil, fmt.Errorf("invalid section payload")
}

func extractJSONPayload(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
			lines = lines[1:]
		}
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
			lines = lines[:len(lines)-1]
		}
		trimmed = strings.Join(lines, "\n")
	}

	trimmed = strings.TrimSpace(trimmed)
	startArray := strings.Index(trimmed, "[")
	startObject := strings.Index(trimmed, "{")
	start := firstPositiveIndex(startArray, startObject)
	if start == -1 {
		return trimmed
	}

	endArray := strings.LastIndex(trimmed, "]")
	endObject := strings.LastIndex(trimmed, "}")
	end := max(endArray, endObject)
	if end <= start {
		return trimmed[start:]
	}

	return strings.TrimSpace(trimmed[start : end+1])
}

func dedupeGeneratedSections(sections []generatedSection) []generatedSection {
	normalized := make([]generatedSection, 0, len(sections))
	seenCases := make(map[string]struct{})

	for _, section := range sections {
		name := strings.TrimSpace(section.Section)
		if name == "" {
			name = "未分类"
		}

		filtered := make([]map[string]any, 0, len(section.Cases))
		for _, item := range section.Cases {
			signature := caseSignature(item)
			if signature == "" {
				continue
			}
			if _, ok := seenCases[signature]; ok {
				continue
			}
			seenCases[signature] = struct{}{}
			filtered = append(filtered, item)
		}
		if len(filtered) == 0 {
			continue
		}

		normalized = append(normalized, generatedSection{
			Section: name,
			Cases:   filtered,
		})
	}

	return normalized
}

func caseSignature(item map[string]any) string {
	title := normalizeMatchText(stringValue(item["title"]))
	preconds := normalizeMatchText(stringValue(item["custom_preconds"]))
	steps := normalizeStepSignatures(item["custom_steps_separated"])

	if title == "" && len(steps) == 0 {
		return ""
	}
	return title + "|" + preconds + "|" + strings.Join(steps, "||")
}

func normalizeStepSignatures(value any) []string {
	signatures := make([]string, 0)
	switch typed := value.(type) {
	case []map[string]any:
		for _, step := range typed {
			content := normalizeMatchText(firstNonEmptyString(step["content"], step["step"]))
			expected := normalizeMatchText(firstNonEmptyString(step["expected"], step["result"]))
			signatures = append(signatures, content+"=>"+expected)
		}
	case []any:
		for _, raw := range typed {
			step, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			content := normalizeMatchText(firstNonEmptyString(step["content"], step["step"]))
			expected := normalizeMatchText(firstNonEmptyString(step["expected"], step["result"]))
			signatures = append(signatures, content+"=>"+expected)
		}
	}
	return signatures
}

func normalizeMatchText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), ""))
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if text := stringValue(value); text != "" {
			return text
		}
	}
	return ""
}

func firstPositiveIndex(values ...int) int {
	best := -1
	for _, value := range values {
		if value < 0 {
			continue
		}
		if best == -1 || value < best {
			best = value
		}
	}
	return best
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
