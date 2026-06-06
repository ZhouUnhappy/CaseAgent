package agent

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"caseagent/internal/agent/prompts"
	"caseagent/internal/ai"
	"caseagent/internal/db/models"
	workflowservice "caseagent/internal/service/workflow"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type modelTraceContextKey string

const modelTraceKey modelTraceContextKey = "agent_model_trace"
const modelTraceCollectorKey modelTraceContextKey = "agent_model_trace_collector"

type modelTraceInfo struct {
	Agent   string
	Attempt string
}

type tracedChatModel struct {
	base         einomodel.BaseChatModel
	recorder     workflowservice.ModelCallRecorder
	provider     string
	modelName    string
	providerRole string
}

type ModelCallTrace struct {
	ID                  int    `json:"id"`
	AgentRunID          int    `json:"agent_run_id,omitempty"`
	Agent               string `json:"agent,omitempty"`
	Attempt             string `json:"attempt,omitempty"`
	Provider            string `json:"provider,omitempty"`
	Model               string `json:"model,omitempty"`
	ProviderRole        string `json:"provider_role,omitempty"`
	Status              string `json:"status"`
	PromptID            string `json:"prompt_id,omitempty"`
	PromptVersion       string `json:"prompt_version,omitempty"`
	PromptChars         int    `json:"prompt_chars"`
	ResponseChars       int    `json:"response_chars"`
	EstimatedTotalToken int    `json:"estimated_total_tokens"`
	TotalTokens         int    `json:"total_tokens,omitempty"`
	TokenSource         string `json:"token_source"`
	LastError           string `json:"last_error,omitempty"`
}

type ModelCallCollector struct {
	mu    sync.Mutex
	calls []ModelCallTrace
}

func NewModelCallCollector() *ModelCallCollector {
	return &ModelCallCollector{}
}

func WithModelCallCollector(ctx context.Context, collector *ModelCallCollector) context.Context {
	if collector == nil {
		return ctx
	}
	return context.WithValue(ctx, modelTraceCollectorKey, collector)
}

func ModelCallsFromContext(ctx context.Context) *ModelCallCollector {
	collector, _ := ctx.Value(modelTraceCollectorKey).(*ModelCallCollector)
	return collector
}

func (c *ModelCallCollector) Add(call ModelCallTrace) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, call)
}

func (c *ModelCallCollector) Calls() []ModelCallTrace {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]ModelCallTrace{}, c.calls...)
}

func traceChatModel(base einomodel.BaseChatModel, recorder workflowservice.ModelCallRecorder, provider string, modelName string, providerRole ...string) einomodel.BaseChatModel {
	if base == nil || recorder == nil {
		return base
	}
	role := "primary"
	if len(providerRole) > 0 && providerRole[0] != "" {
		role = providerRole[0]
	}
	return &tracedChatModel{
		base:         base,
		recorder:     recorder,
		provider:     provider,
		modelName:    modelName,
		providerRole: role,
	}
}

func withModelTrace(ctx context.Context, agentName string, attempt string) context.Context {
	return context.WithValue(ctx, modelTraceKey, modelTraceInfo{
		Agent:   agentName,
		Attempt: attempt,
	})
}

func (m *tracedChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	started := time.Now()
	result, err := m.base.Generate(ctx, input, opts...)
	m.record(ctx, input, result, err, false, time.Since(started))
	return result, err
}

func (m *tracedChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	started := time.Now()
	reader, err := m.base.Stream(ctx, input, opts...)
	m.record(ctx, input, nil, err, true, time.Since(started))
	return reader, err
}

func (m *tracedChatModel) record(ctx context.Context, input []*schema.Message, output *schema.Message, cause error, streaming bool, elapsed time.Duration) {
	status := models.WorkflowStatusSucceeded
	lastErr := ""
	if cause != nil {
		status = models.WorkflowStatusFailed
		lastErr = cause.Error()
	}

	traceInfo, _ := ctx.Value(modelTraceKey).(modelTraceInfo)
	metadata := map[string]any{
		"agent":      traceInfo.Agent,
		"attempt":    traceInfo.Attempt,
		"elapsed_ms": elapsed.Milliseconds(),
		"streaming":  streaming,
	}
	if promptInfo, ok := prompts.TraceFromContext(ctx); ok {
		metadata["prompt_id"] = string(promptInfo.ID)
		metadata["prompt_version"] = promptInfo.Version
	}
	metadata["provider_role"] = m.providerRole
	if fallbackCause, ok := ctx.Value(fallbackCauseKey).(map[string]any); ok {
		metadata["fallback"] = fallbackCause
	}
	usage := ai.EstimateMessageUsage(input, output)
	metadata["cost"] = ai.UsageMetadata(usage)
	if output != nil && output.ResponseMeta != nil {
		metadata["finish_reason"] = output.ResponseMeta.FinishReason
		if output.ResponseMeta.Usage != nil {
			metadata["usage"] = output.ResponseMeta.Usage
		}
	}

	row, err := m.recorder.RecordModelCall(ctx, workflowservice.ModelCallInput{
		WorkflowRunID: workflowservice.RunIDPointerFromContext(ctx),
		AgentRunID:    workflowservice.AgentRunIDPointerFromContext(ctx),
		Provider:      m.provider,
		Model:         m.modelName,
		Status:        status,
		PromptChars:   usage.PromptChars,
		ResponseChars: usage.CompletionChars,
		LastError:     lastErr,
		Metadata:      metadata,
	})
	if err != nil {
		slog.Warn("model call trace record failed",
			"agent", traceInfo.Agent,
			"attempt", traceInfo.Attempt,
			"error", err)
		return
	}
	if collector := ModelCallsFromContext(ctx); collector != nil {
		collector.Add(modelCallTraceFromRow(row, traceInfo, metadata, usage, lastErr, m.providerRole))
	}
}

func messageChars(messages []*schema.Message) int {
	return ai.MessageChars(messages)
}

func modelCallTraceFromRow(row *models.ModelCall, traceInfo modelTraceInfo, metadata map[string]any, usage ai.UsageEstimate, lastErr string, providerRole string) ModelCallTrace {
	ref := ModelCallTrace{
		Agent:               traceInfo.Agent,
		Attempt:             traceInfo.Attempt,
		ProviderRole:        providerRole,
		Status:              models.WorkflowStatusSucceeded,
		PromptChars:         usage.PromptChars,
		ResponseChars:       usage.CompletionChars,
		EstimatedTotalToken: usage.EstimatedTotalTokens,
		TotalTokens:         usage.TotalTokens,
		TokenSource:         usage.TokenSource,
		LastError:           lastErr,
	}
	if row != nil {
		ref.ID = row.ID
		ref.Provider = row.Provider
		ref.Model = row.Model
		ref.Status = row.Status
		if row.AgentRunID != nil {
			ref.AgentRunID = *row.AgentRunID
		}
	}
	if promptID, ok := metadata["prompt_id"].(string); ok {
		ref.PromptID = promptID
	}
	if promptVersion, ok := metadata["prompt_version"].(string); ok {
		ref.PromptVersion = promptVersion
	}
	return ref
}
