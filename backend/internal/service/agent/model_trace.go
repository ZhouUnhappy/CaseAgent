package agent

import (
	"context"
	"log/slog"
	"time"

	"caseagent/internal/agent/prompts"
	"caseagent/internal/db/models"
	workflowservice "caseagent/internal/service/workflow"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type modelTraceContextKey string

const modelTraceKey modelTraceContextKey = "agent_model_trace"

type modelTraceInfo struct {
	Agent   string
	Attempt string
}

type tracedChatModel struct {
	base      einomodel.BaseChatModel
	recorder  workflowservice.ModelCallRecorder
	provider  string
	modelName string
}

func traceChatModel(base einomodel.BaseChatModel, recorder workflowservice.ModelCallRecorder, provider string, modelName string) einomodel.BaseChatModel {
	if base == nil || recorder == nil {
		return base
	}
	return &tracedChatModel{
		base:      base,
		recorder:  recorder,
		provider:  provider,
		modelName: modelName,
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
	if output != nil && output.ResponseMeta != nil {
		metadata["finish_reason"] = output.ResponseMeta.FinishReason
		if output.ResponseMeta.Usage != nil {
			metadata["usage"] = output.ResponseMeta.Usage
		}
	}

	if _, err := m.recorder.RecordModelCall(ctx, workflowservice.ModelCallInput{
		WorkflowRunID: workflowservice.RunIDPointerFromContext(ctx),
		AgentRunID:    workflowservice.AgentRunIDPointerFromContext(ctx),
		Provider:      m.provider,
		Model:         m.modelName,
		Status:        status,
		PromptChars:   messageChars(input),
		ResponseChars: messageChars([]*schema.Message{output}),
		LastError:     lastErr,
		Metadata:      metadata,
	}); err != nil {
		slog.Warn("model call trace record failed",
			"agent", traceInfo.Agent,
			"attempt", traceInfo.Attempt,
			"error", err)
	}
}

func messageChars(messages []*schema.Message) int {
	total := 0
	for _, message := range messages {
		if message == nil {
			continue
		}
		total += len(message.Content)
		total += len(message.ReasoningContent)
		for _, part := range message.MultiContent {
			total += len(part.Text)
		}
		for _, part := range message.UserInputMultiContent {
			total += len(part.Text)
		}
		for _, part := range message.AssistantGenMultiContent {
			total += len(part.Text)
		}
	}
	return total
}
