package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"caseagent/internal/ai"
	"caseagent/internal/db/models"
	workflowservice "caseagent/internal/service/workflow"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const (
	GuardrailEventBudgetExceeded = "budget_exceeded"
	GuardrailEventCircuitOpen    = "circuit_open"
)

var (
	ErrModelBudgetExceeded = errors.New("model task budget exhausted")
	ErrModelCircuitOpen    = errors.New("model provider circuit breaker open")
)

func IsTerminalGuardrailError(err error) bool {
	return errors.Is(err, ErrModelBudgetExceeded) || errors.Is(err, ErrModelCircuitOpen)
}

type GuardrailConfig struct {
	TaskBudgetTokens        int
	ProviderTimeout         time.Duration
	FailureThreshold        int
	CircuitOpenCooldown     time.Duration
	FallbackChatModel       einomodel.BaseChatModel
	FallbackProvider        string
	FallbackModelName       string
	FallbackProviderTimeout time.Duration
}

type guardedChatModel struct {
	primary         einomodel.BaseChatModel
	fallback        einomodel.BaseChatModel
	recorder        workflowservice.ModelCallRecorder
	provider        string
	modelName       string
	providerTimeout time.Duration
	fallbackTimeout time.Duration
	state           *guardrailState
}

type guardrailState struct {
	mu                  sync.Mutex
	taskBudgetTokens    int
	usedTokens          int
	failureThreshold    int
	circuitOpenCooldown time.Duration
	consecutiveFailures int
	openedUntil         time.Time
	now                 func() time.Time
}

func guardChatModel(primary einomodel.BaseChatModel, recorder workflowservice.ModelCallRecorder, provider string, modelName string, cfg GuardrailConfig) einomodel.BaseChatModel {
	if primary == nil {
		return nil
	}
	if !guardrailEnabled(cfg) {
		return primary
	}
	state := &guardrailState{
		taskBudgetTokens:    cfg.TaskBudgetTokens,
		failureThreshold:    cfg.FailureThreshold,
		circuitOpenCooldown: cfg.CircuitOpenCooldown,
		now:                 time.Now,
	}
	return &guardedChatModel{
		primary:         primary,
		fallback:        cfg.FallbackChatModel,
		recorder:        recorder,
		provider:        provider,
		modelName:       modelName,
		providerTimeout: cfg.ProviderTimeout,
		fallbackTimeout: cfg.FallbackProviderTimeout,
		state:           state,
	}
}

func guardrailEnabled(cfg GuardrailConfig) bool {
	return cfg.TaskBudgetTokens > 0 ||
		cfg.ProviderTimeout > 0 ||
		cfg.FailureThreshold > 0 ||
		cfg.FallbackChatModel != nil
}

func (m *guardedChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	if err := m.checkBudget(ctx, input); err != nil {
		return nil, err
	}
	if err := m.checkCircuit(ctx, input); err != nil {
		if m.fallback == nil {
			return nil, err
		}
		return m.generateFallback(ctx, input, opts...)
	}

	primaryCtx, cancel := providerContext(ctx, m.providerTimeout)
	started := time.Now()
	result, err := m.primary.Generate(primaryCtx, input, opts...)
	cancel()
	m.account(input, result)
	if err == nil {
		m.state.recordSuccess()
		return result, nil
	}
	if primaryCtx.Err() != nil && !errors.Is(ctx.Err(), primaryCtx.Err()) {
		err = primaryCtx.Err()
	}
	m.state.recordFailure()
	if m.fallback == nil || ctx.Err() != nil {
		return nil, err
	}
	return m.generateFallback(withFallbackCause(ctx, err, time.Since(started)), input, opts...)
}

func (m *guardedChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	if err := m.checkBudget(ctx, input); err != nil {
		return nil, err
	}
	if err := m.checkCircuit(ctx, input); err != nil {
		if m.fallback == nil {
			return nil, err
		}
		return m.streamFallback(ctx, input, opts...)
	}

	primaryCtx, cancel := providerContext(ctx, m.providerTimeout)
	reader, err := m.primary.Stream(primaryCtx, input, opts...)
	cancel()
	m.account(input, nil)
	if err == nil {
		m.state.recordSuccess()
		return reader, nil
	}
	if primaryCtx.Err() != nil && !errors.Is(ctx.Err(), primaryCtx.Err()) {
		err = primaryCtx.Err()
	}
	m.state.recordFailure()
	if m.fallback == nil || ctx.Err() != nil {
		return nil, err
	}
	return m.streamFallback(withFallbackCause(ctx, err, 0), input, opts...)
}

func (m *guardedChatModel) generateFallback(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	if err := m.checkBudget(ctx, input); err != nil {
		return nil, err
	}
	fallbackCtx, cancel := providerContext(ctx, m.fallbackTimeout)
	result, err := m.fallback.Generate(fallbackCtx, input, opts...)
	cancel()
	m.account(input, result)
	if err != nil && fallbackCtx.Err() != nil && !errors.Is(ctx.Err(), fallbackCtx.Err()) {
		err = fallbackCtx.Err()
	}
	return result, err
}

func (m *guardedChatModel) streamFallback(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	if err := m.checkBudget(ctx, input); err != nil {
		return nil, err
	}
	fallbackCtx, cancel := providerContext(ctx, m.fallbackTimeout)
	reader, err := m.fallback.Stream(fallbackCtx, input, opts...)
	cancel()
	m.account(input, nil)
	if err != nil && fallbackCtx.Err() != nil && !errors.Is(ctx.Err(), fallbackCtx.Err()) {
		err = fallbackCtx.Err()
	}
	return reader, err
}

func (m *guardedChatModel) checkBudget(ctx context.Context, input []*schema.Message) error {
	used, budget, promptTokens, ok := m.state.checkBudget(input)
	if ok {
		return nil
	}
	err := fmt.Errorf("%w: used=%d prompt_estimate=%d budget=%d", ErrModelBudgetExceeded, used, promptTokens, budget)
	m.recordGuardrailFailure(ctx, input, GuardrailEventBudgetExceeded, err, map[string]any{
		"budget_tokens":           budget,
		"used_tokens_before":      used,
		"prompt_estimated_tokens": promptTokens,
	})
	return err
}

func (m *guardedChatModel) checkCircuit(ctx context.Context, input []*schema.Message) error {
	openedUntil, ok := m.state.checkCircuit()
	if ok {
		return nil
	}
	err := fmt.Errorf("%w: provider=%s model=%s open_until=%s", ErrModelCircuitOpen, m.provider, m.modelName, openedUntil.Format(time.RFC3339))
	m.recordGuardrailFailure(ctx, input, GuardrailEventCircuitOpen, err, map[string]any{
		"opened_until":       openedUntil.Format(time.RFC3339),
		"fallback_available": m.fallback != nil,
	})
	return err
}

func (m *guardedChatModel) account(input []*schema.Message, output *schema.Message) {
	usage := ai.EstimateMessageUsage(input, output)
	m.state.addUsage(ai.AccountedTokens(usage))
}

func (m *guardedChatModel) recordGuardrailFailure(ctx context.Context, input []*schema.Message, event string, cause error, metadata map[string]any) {
	if m.recorder == nil {
		return
	}
	traceInfo, _ := ctx.Value(modelTraceKey).(modelTraceInfo)
	usage := ai.EstimateUsage(ai.MessageChars(input), 0, nil)
	payload := mergeMetadata(map[string]any{
		"agent":           traceInfo.Agent,
		"attempt":         traceInfo.Attempt,
		"provider_role":   "primary",
		"guardrail_event": event,
		"cost":            ai.UsageMetadata(usage),
	}, metadata)
	row, err := m.recorder.RecordModelCall(ctx, workflowservice.ModelCallInput{
		WorkflowRunID: workflowservice.RunIDPointerFromContext(ctx),
		AgentRunID:    workflowservice.AgentRunIDPointerFromContext(ctx),
		Provider:      m.provider,
		Model:         m.modelName,
		Status:        models.WorkflowStatusFailed,
		PromptChars:   usage.PromptChars,
		ResponseChars: 0,
		LastError:     cause.Error(),
		Metadata:      payload,
	})
	if err != nil {
		return
	}
	if collector := ModelCallsFromContext(ctx); collector != nil {
		collector.Add(modelCallTraceFromRow(row, traceInfo, payload, usage, cause.Error(), "primary"))
	}
}

func (s *guardrailState) checkBudget(input []*schema.Message) (used int, budget int, promptTokens int, ok bool) {
	if s == nil || s.taskBudgetTokens <= 0 {
		return 0, 0, 0, true
	}
	promptTokens = ai.EstimatePromptTokens(input)
	s.mu.Lock()
	defer s.mu.Unlock()
	used = s.usedTokens
	budget = s.taskBudgetTokens
	return used, budget, promptTokens, used+promptTokens <= s.taskBudgetTokens
}

func (s *guardrailState) addUsage(tokens int) {
	if s == nil || tokens <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usedTokens += tokens
}

func (s *guardrailState) checkCircuit() (time.Time, bool) {
	if s == nil || s.failureThreshold <= 0 {
		return time.Time{}, true
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.openedUntil.IsZero() || !now.Before(s.openedUntil) {
		return time.Time{}, true
	}
	return s.openedUntil, false
}

func (s *guardrailState) recordSuccess() {
	if s == nil || s.failureThreshold <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consecutiveFailures = 0
	s.openedUntil = time.Time{}
}

func (s *guardrailState) recordFailure() {
	if s == nil || s.failureThreshold <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consecutiveFailures++
	if s.consecutiveFailures >= s.failureThreshold {
		cooldown := s.circuitOpenCooldown
		if cooldown <= 0 {
			cooldown = time.Minute
		}
		s.openedUntil = s.now().Add(cooldown)
	}
}

func providerContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

type fallbackCauseContextKey string

const fallbackCauseKey fallbackCauseContextKey = "guardrail_fallback_cause"

func withFallbackCause(ctx context.Context, cause error, elapsed time.Duration) context.Context {
	if cause == nil {
		return ctx
	}
	return context.WithValue(ctx, fallbackCauseKey, map[string]any{
		"primary_error":      cause.Error(),
		"primary_elapsed_ms": elapsed.Milliseconds(),
	})
}
