// Package agent owns the orchestration boundary between the application's
// generation pipeline and the eino agent layer.
//
// Responsibility split:
//
//   - Service (this package) is the *application-level coordinator*. It owns
//     the sub-agent instances (functional / ops / failure / boundary), runs
//     them sequentially, applies retry-once on transient failures, isolates
//     individual sub-agent failures so they don't block sibling agents,
//     dedupes the merged sections, and finally hands the consolidated draft
//     to DeepAgent for refinement. Service does not call the LLM directly.
//
//   - DeepAgent (backend/internal/agent/deep) is the *agent-level coordinator*
//     that talks to the chat model. It serves two roles: (a) fallback —
//     when every sub-agent fails or produces no parseable output, DeepAgent
//     generates a full set of sections from scratch; (b) refinement —
//     consolidating the dedup'd sub-agent draft into a single coherent
//     output before persistence.
//
// Sub-agents are intentionally not yet wired through DeepAgent's adk.Agent
// slot; the current architecture is sequential rather than graph-based, and
// a follow-up will migrate to native eino adk.Agent coordination.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"caseagent/internal/agent/boundary"
	"caseagent/internal/agent/deep"
	"caseagent/internal/agent/failure"
	"caseagent/internal/agent/functional"
	"caseagent/internal/agent/ops"
	"caseagent/internal/config"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
)

type Service struct {
	deepAgent       *deep.Agent
	functionalAgent *functional.Agent
	opsAgent        *ops.Agent
	failureAgent    *failure.Agent
	boundaryAgent   *boundary.Agent
}

type Config struct {
	ChatModel model.BaseChatModel // Optional, if not provided will initialize from config
}

func New(ctx context.Context, cfg *Config) (*Service, error) {
	var chatModel model.BaseChatModel
	var err error

	// If ChatModel is provided, use it; otherwise initialize from config
	if cfg.ChatModel != nil {
		chatModel = cfg.ChatModel
	} else {
		// Initialize chat model from config
		appCfg := config.Get()
		if appCfg.Model.Chat.Provider != "ark" {
			return nil, fmt.Errorf("only ark chat model provider is supported, got: %s", appCfg.Model.Chat.Provider)
		}

		chatModel, err = ark.NewChatModel(ctx, &ark.ChatModelConfig{
			APIKey:    appCfg.Model.Chat.APIKey,
			AccessKey: appCfg.Model.Chat.AccessKey,
			SecretKey: appCfg.Model.Chat.SecretKey,
			BaseURL:   appCfg.Model.Chat.BaseURL,
			Region:    appCfg.Model.Chat.Region,
			Model:     appCfg.Model.Chat.Model,
		})

		if err != nil {
			return nil, fmt.Errorf("failed to initialize chat model: %w", err)
		}
	}

	// Create sub-agents
	functionalAgent, err := functional.New(ctx, &functional.Config{ChatModel: chatModel})
	if err != nil {
		return nil, fmt.Errorf("failed to create functional agent: %w", err)
	}

	opsAgent, err := ops.New(ctx, &ops.Config{ChatModel: chatModel})
	if err != nil {
		return nil, fmt.Errorf("failed to create ops agent: %w", err)
	}

	failureAgent, err := failure.New(ctx, &failure.Config{ChatModel: chatModel})
	if err != nil {
		return nil, fmt.Errorf("failed to create failure agent: %w", err)
	}

	boundaryAgent, err := boundary.New(ctx, &boundary.Config{ChatModel: chatModel})
	if err != nil {
		return nil, fmt.Errorf("failed to create boundary agent: %w", err)
	}

	// Create DeepAgent with sub-agents
	// TODO: Convert sub-agents to adk.Agent interface
	var subAgents []adk.Agent

	deepAgent, err := deep.New(ctx, &deep.Config{
		ChatModel: chatModel,
		SubAgents: subAgents,
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
	}, nil
}

// GenerateCases runs each sub-agent (with retry-once on transient failure),
// dedupes the merged sections, and asks DeepAgent to refine the consolidated
// draft. A single sub-agent failing — even after its retry — is logged and
// skipped; sibling sub-agents continue running and their outputs still flow
// through dedupe → refinement → persistence.
func (s *Service) GenerateCases(ctx context.Context, requirements string, knowledge string) (string, error) {
	type generator struct {
		name string
		run  func(context.Context, string, string) (string, error)
	}

	generators := []generator{
		{name: "functional", run: s.functionalAgent.GenerateFunctionalCases},
		{name: "ops", run: s.opsAgent.GenerateOpsCases},
		{name: "failure", run: s.failureAgent.GenerateFailureCases},
		{name: "boundary", run: s.boundaryAgent.GenerateBoundaryCases},
	}

	sections := make([]generatedSection, 0, len(generators))
	for _, g := range generators {
		output, err := runSubAgentWithRetry(ctx, g.name, func(ctx context.Context) (string, error) {
			return g.run(ctx, requirements, knowledge)
		})
		if err != nil {
			slog.Warn("sub-agent failed after retry, continuing without it",
				"agent", g.name, "error", err)
			continue
		}

		parsed, parseErr := parseGeneratedSections(output)
		if parseErr != nil || len(parsed) == 0 {
			slog.Warn("sub-agent produced unparseable output, skipping",
				"agent", g.name, "parse_err", parseErr, "len", len(parsed))
			continue
		}
		sections = append(sections, parsed...)
	}

	if len(sections) == 0 {
		slog.Warn("all sub-agents produced no usable output; falling back to DeepAgent.GenerateCases",
			"sub_agent_count", len(generators))
		return s.deepAgent.GenerateCases(ctx, requirements, knowledge)
	}

	normalized := dedupeGeneratedSections(sections)
	if len(normalized) == 0 {
		slog.Warn("dedupe collapsed all sub-agent sections; falling back to DeepAgent.GenerateCases")
		return s.deepAgent.GenerateCases(ctx, requirements, knowledge)
	}

	payload, err := json.Marshal(normalized)
	if err != nil {
		slog.Warn("failed to marshal dedup'd sections; falling back to DeepAgent.GenerateCases", "error", err)
		return s.deepAgent.GenerateCases(ctx, requirements, knowledge)
	}

	refined, err := s.deepAgent.RefineCases(ctx, requirements, knowledge, string(payload))
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

// runSubAgentWithRetry invokes fn once and, on error, retries exactly one
// more time. It logs the first-attempt error so that intermittent provider
// failures (rate limits, transient 5xx) are visible in operator logs.
func runSubAgentWithRetry(ctx context.Context, name string, fn func(context.Context) (string, error)) (string, error) {
	output, err := fn(ctx)
	if err == nil {
		return output, nil
	}
	slog.Warn("sub-agent first attempt failed; retrying once", "agent", name, "error", err)
	return fn(ctx)
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
