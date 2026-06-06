package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"caseagent/internal/agent/prompts"
	"caseagent/internal/ai"
	"caseagent/internal/config"
	"caseagent/internal/db/models"
	workflowservice "caseagent/internal/service/workflow"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func TestParseGeneratedSections(t *testing.T) {
	raw := "```json\n[\n  {\n    \"section\": \"功能测试\",\n    \"cases\": [\n      {\n        \"title\": \"验证创建成功\",\n        \"custom_preconds\": \"服务已启动\",\n        \"custom_steps_separated\": [\n          {\"content\": \"提交请求\", \"expected\": \"成功\"}\n        ]\n      }\n    ]\n  }\n]\n```"

	sections, err := parseGeneratedSections(raw)
	if err != nil {
		t.Fatalf("parseGeneratedSections returned error: %v", err)
	}
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
	if sections[0].Section != "功能测试" {
		t.Fatalf("unexpected section: %q", sections[0].Section)
	}
}

func TestDedupeGeneratedSections(t *testing.T) {
	input := []generatedSection{
		{
			Section: "功能测试",
			Cases: []map[string]any{
				{
					"title":           "验证创建成功",
					"custom_preconds": "服务已启动",
					"custom_steps_separated": []map[string]any{
						{"content": "提交请求", "expected": "成功"},
					},
				},
			},
		},
		{
			Section: "边界测试",
			Cases: []map[string]any{
				{
					"title":           "验证创建成功",
					"custom_preconds": "服务已启动",
					"custom_steps_separated": []map[string]any{
						{"content": "提交请求", "expected": "成功"},
					},
				},
			},
		},
	}

	got := dedupeGeneratedSections(input)
	if len(got) != 1 {
		t.Fatalf("expected one section after dedupe, got %d", len(got))
	}
	if len(got[0].Cases) != 1 {
		t.Fatalf("expected one case after dedupe, got %d", len(got[0].Cases))
	}
}

func TestMessageCharsCountsPromptAndReasoningContent(t *testing.T) {
	got := messageChars([]*schema.Message{
		schema.UserMessage("abc"),
		{
			Content:          "de",
			ReasoningContent: "fg",
		},
		nil,
	})
	if got != 7 {
		t.Fatalf("messageChars() = %d, want 7", got)
	}
}

func TestRunTimedAgentCallLinksModelCallsToAgentRun(t *testing.T) {
	recorder := &fakeAgentTraceRecorder{nextAgentRunID: 77}
	chatModel := traceChatModel(&stubChatModel{content: "ok"}, recorder, "fake", "trace")
	rendered := prompts.Rendered{ID: prompts.FunctionalCases, Version: "v1", Content: "prompt"}

	output, err := runTimedAgentCall(context.Background(), "functional", "initial", time.Second, recorder, func(ctx context.Context) (string, error) {
		result, err := chatModel.Generate(prompts.WithRenderedPrompt(ctx, rendered), []*schema.Message{schema.UserMessage(rendered.Content)})
		if err != nil {
			return "", err
		}
		return result.Content, nil
	})
	if err != nil {
		t.Fatalf("runTimedAgentCall() returned error: %v", err)
	}
	if output != "ok" {
		t.Fatalf("output = %q, want ok", output)
	}
	if len(recorder.startedAgents) != 1 || recorder.startedAgents[0].AgentName != "functional" {
		t.Fatalf("started agents = %#v", recorder.startedAgents)
	}
	if len(recorder.modelCalls) != 1 {
		t.Fatalf("model calls = %#v, want one", recorder.modelCalls)
	}
	if recorder.modelCalls[0].AgentRunID == nil || *recorder.modelCalls[0].AgentRunID != 77 {
		t.Fatalf("model call agent_run_id = %#v, want 77", recorder.modelCalls[0].AgentRunID)
	}
	if recorder.modelCalls[0].Metadata["prompt_id"] != string(prompts.FunctionalCases) {
		t.Fatalf("model call prompt_id = %#v", recorder.modelCalls[0].Metadata["prompt_id"])
	}
	if recorder.modelCalls[0].Metadata["prompt_version"] != "v1" {
		t.Fatalf("model call prompt_version = %#v", recorder.modelCalls[0].Metadata["prompt_version"])
	}
	if len(recorder.finishedAgents) != 1 || recorder.finishedAgents[0].Status != models.WorkflowStatusSucceeded {
		t.Fatalf("finished agents = %#v", recorder.finishedAgents)
	}
}

func TestRunSubAgentWithRetry(t *testing.T) {
	ctx := context.Background()

	t.Run("success on first try makes only one call", func(t *testing.T) {
		calls := 0
		out, err := runSubAgentWithRetry(ctx, "ok", time.Second, nil, func(_ context.Context) (string, error) {
			calls++
			return "payload", nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out != "payload" {
			t.Fatalf("unexpected output %q", out)
		}
		if calls != 1 {
			t.Fatalf("expected 1 call when first try succeeds, got %d", calls)
		}
	})

	t.Run("transient failure recovered on retry", func(t *testing.T) {
		calls := 0
		out, err := runSubAgentWithRetry(ctx, "transient", time.Second, nil, func(_ context.Context) (string, error) {
			calls++
			if calls == 1 {
				return "", errors.New("rate limited")
			}
			return "payload-2", nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out != "payload-2" {
			t.Fatalf("unexpected output %q", out)
		}
		if calls != 2 {
			t.Fatalf("expected exactly 2 calls (try + 1 retry), got %d", calls)
		}
	})

	t.Run("persistent failure surfaces after exactly one retry", func(t *testing.T) {
		calls := 0
		_, err := runSubAgentWithRetry(ctx, "persistent", time.Second, nil, func(_ context.Context) (string, error) {
			calls++
			return "", errors.New("hard fail")
		})
		if err == nil {
			t.Fatal("expected error after exhausting retry")
		}
		if calls != 2 {
			t.Fatalf("expected exactly 2 calls (try + 1 retry) before giving up, got %d", calls)
		}
	})

	t.Run("deadline exceeded is not retried", func(t *testing.T) {
		calls := 0
		_, err := runSubAgentWithRetry(ctx, "timeout", time.Nanosecond, nil, func(ctx context.Context) (string, error) {
			calls++
			<-ctx.Done()
			return "", ctx.Err()
		})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline exceeded, got %v", err)
		}
		if calls != 1 {
			t.Fatalf("expected no retry for deadline exceeded, got %d calls", calls)
		}
	})
}

type fakeAgentTraceRecorder struct {
	nextAgentRunID int
	startedAgents  []workflowservice.StartAgentRunInput
	finishedAgents []workflowservice.FinishAgentRunInput
	modelCalls     []workflowservice.ModelCallInput
}

func (r *fakeAgentTraceRecorder) StartAgentRun(ctx context.Context, input workflowservice.StartAgentRunInput) (*models.AgentRun, error) {
	r.startedAgents = append(r.startedAgents, input)
	id := r.nextAgentRunID
	if id <= 0 {
		id = len(r.startedAgents)
	}
	return &models.AgentRun{ID: id}, nil
}

func (r *fakeAgentTraceRecorder) FinishAgentRun(ctx context.Context, agentRunID int, input workflowservice.FinishAgentRunInput) error {
	r.finishedAgents = append(r.finishedAgents, input)
	return nil
}

func (r *fakeAgentTraceRecorder) RecordModelCall(ctx context.Context, input workflowservice.ModelCallInput) (*models.ModelCall, error) {
	r.modelCalls = append(r.modelCalls, input)
	return &models.ModelCall{ID: len(r.modelCalls)}, nil
}

type stubChatModel struct {
	content string
	err     error
}

func (m *stubChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	if m.err != nil {
		return nil, m.err
	}
	return schema.AssistantMessage(m.content, nil), nil
}

func (m *stubChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("stream not implemented")
}

type routingChatModel struct {
	generate func([]*schema.Message) (*schema.Message, error)
}

func (m *routingChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	return m.generate(input)
}

func (m *routingChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("stream not implemented")
}

func TestConfiguredChatCallTimeout(t *testing.T) {
	if got := configuredChatCallTimeout(nil); got != defaultChatCallTimeout {
		t.Fatalf("nil config timeout = %s, want %s", got, defaultChatCallTimeout)
	}
	cfg := &config.Config{}
	if got := configuredChatCallTimeout(cfg); got != defaultChatCallTimeout {
		t.Fatalf("zero config timeout = %s, want %s", got, defaultChatCallTimeout)
	}
	cfg.Model.Chat.RequestTimeoutSeconds = 7
	if got := configuredChatCallTimeout(cfg); got != 7*time.Second {
		t.Fatalf("configured timeout = %s, want 7s", got)
	}
}

func TestGenerationErrorCarriesFailureStage(t *testing.T) {
	cause := errors.New("provider rejected request")
	err := generationError(GenerationStageDeepAgentFallback, cause)
	if err == nil {
		t.Fatal("expected generation error")
	}
	if FailureStage(err) != GenerationStageDeepAgentFallback {
		t.Fatalf("FailureStage() = %q, want %q", FailureStage(err), GenerationStageDeepAgentFallback)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("generation error should unwrap original cause")
	}
}

func TestGenerateCasesWithFakeProvider(t *testing.T) {
	chatModel, err := ai.NewChatModel(context.Background(), config.ChatModelConfig{
		Provider: "fake",
		Model:    "valid_json",
	})
	if err != nil {
		t.Fatalf("NewChatModel() returned error: %v", err)
	}
	service, err := New(context.Background(), &Config{ChatModel: chatModel, ChatCallTimeout: time.Second})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	output, err := service.GenerateCases(context.Background(), "fake requirements", "fake knowledge")
	if err != nil {
		t.Fatalf("GenerateCases() returned error: %v", err)
	}
	if !strings.Contains(output, `"section": "功能测试"`) {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestGenerateCasesWithFakePartialFailureProvider(t *testing.T) {
	chatModel, err := ai.NewChatModel(context.Background(), config.ChatModelConfig{
		Provider: "fake",
		Model:    "partial_failure",
	})
	if err != nil {
		t.Fatalf("NewChatModel() returned error: %v", err)
	}
	service, err := New(context.Background(), &Config{ChatModel: chatModel, ChatCallTimeout: time.Second})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	output, err := service.GenerateCases(context.Background(), "fake requirements", "fake knowledge")
	if err != nil {
		t.Fatalf("GenerateCases() returned error despite partial failure: %v", err)
	}
	if !strings.Contains(output, "功能测试") {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestGenerateCasesWithFakeTimeoutProvider(t *testing.T) {
	chatModel, err := ai.NewChatModel(context.Background(), config.ChatModelConfig{
		Provider: "fake",
		Model:    "timeout",
	})
	if err != nil {
		t.Fatalf("NewChatModel() returned error: %v", err)
	}
	service, err := New(context.Background(), &Config{ChatModel: chatModel, ChatCallTimeout: time.Millisecond})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	_, err = service.GenerateCases(context.Background(), "fake requirements", "fake knowledge")
	if err == nil {
		t.Fatal("GenerateCases() expected timeout error")
	}
	if FailureStage(err) != GenerationStageDeepAgentFallback {
		t.Fatalf("FailureStage() = %q, want %q", FailureStage(err), GenerationStageDeepAgentFallback)
	}
}

func TestGenerateCasesUsesDeepFallbackWhenAllGraphNodesFail(t *testing.T) {
	chatModel := &routingChatModel{generate: func(input []*schema.Message) (*schema.Message, error) {
		prompt := joinedPrompt(input)
		if strings.Contains(prompt, "测试用例生成专家") {
			return schema.AssistantMessage(testSectionJSON("功能测试", "deep fallback"), nil), nil
		}
		return nil, errors.New("sub-agent unavailable")
	}}
	service, err := New(context.Background(), &Config{ChatModel: chatModel, ChatCallTimeout: time.Second})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	output, err := service.GenerateCases(context.Background(), "fake requirements", "fake knowledge")
	if err != nil {
		t.Fatalf("GenerateCases() returned error: %v", err)
	}
	if !strings.Contains(output, "deep fallback") {
		t.Fatalf("output = %s, want DeepAgent fallback content", output)
	}
}

func TestGenerateCasesReturnsUnrefinedPayloadWhenRefineFails(t *testing.T) {
	chatModel := &routingChatModel{generate: func(input []*schema.Message) (*schema.Message, error) {
		prompt := joinedPrompt(input)
		if strings.Contains(prompt, "总协调 Agent") {
			return nil, errors.New("refine failed")
		}
		switch {
		case strings.Contains(prompt, "运维测试专家"):
			return schema.AssistantMessage(testSectionJSON("运维测试", "ops draft"), nil), nil
		case strings.Contains(prompt, "故障测试专家"):
			return schema.AssistantMessage(testSectionJSON("故障测试", "failure draft"), nil), nil
		case strings.Contains(prompt, "边界测试专家"):
			return schema.AssistantMessage(testSectionJSON("边界测试", "boundary draft"), nil), nil
		default:
			return schema.AssistantMessage(testSectionJSON("功能测试", "functional draft"), nil), nil
		}
	}}
	service, err := New(context.Background(), &Config{ChatModel: chatModel, ChatCallTimeout: time.Second})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	output, err := service.GenerateCases(context.Background(), "fake requirements", "fake knowledge")
	if err != nil {
		t.Fatalf("GenerateCases() returned error: %v", err)
	}
	if !strings.Contains(output, "functional draft") || !strings.Contains(output, "boundary draft") {
		t.Fatalf("output = %s, want unrefined sub-agent drafts", output)
	}
}

func TestGuardedChatModelBudgetExhaustedShortCircuits(t *testing.T) {
	recorder := &fakeAgentTraceRecorder{}
	primary := &countingChatModel{content: "ok"}
	guarded := guardChatModel(primary, recorder, "fake", "primary", GuardrailConfig{TaskBudgetTokens: 1})

	_, err := guarded.Generate(withModelTrace(context.Background(), "functional", "initial"), []*schema.Message{schema.UserMessage("this prompt is longer than one token")})
	if !errors.Is(err, ErrModelBudgetExceeded) {
		t.Fatalf("Generate() error = %v, want budget exhausted", err)
	}
	if primary.calls() != 0 {
		t.Fatalf("primary calls = %d, want 0", primary.calls())
	}
	if len(recorder.modelCalls) != 1 {
		t.Fatalf("model calls = %#v, want guardrail failure row", recorder.modelCalls)
	}
	if recorder.modelCalls[0].Metadata["guardrail_event"] != GuardrailEventBudgetExceeded {
		t.Fatalf("guardrail event = %#v", recorder.modelCalls[0].Metadata["guardrail_event"])
	}
}

func TestGuardedChatModelProviderTimeout(t *testing.T) {
	primary := &blockingChatModel{}
	guarded := guardChatModel(primary, nil, "fake", "slow", GuardrailConfig{ProviderTimeout: time.Millisecond})

	started := time.Now()
	_, err := guarded.Generate(context.Background(), []*schema.Message{schema.UserMessage("prompt")})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Generate() error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("provider timeout took too long: %s", elapsed)
	}
}

func TestGuardedChatModelFallbackSuccess(t *testing.T) {
	primary := &countingChatModel{err: errors.New("primary unavailable")}
	fallback := &countingChatModel{content: "fallback ok"}
	guarded := guardChatModel(primary, nil, "fake", "primary", GuardrailConfig{FallbackChatModel: fallback})

	result, err := guarded.Generate(context.Background(), []*schema.Message{schema.UserMessage("prompt")})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Content != "fallback ok" {
		t.Fatalf("result = %q, want fallback output", result.Content)
	}
	if primary.calls() != 1 || fallback.calls() != 1 {
		t.Fatalf("calls primary=%d fallback=%d, want 1/1", primary.calls(), fallback.calls())
	}
}

func TestGuardedChatModelFallbackFailure(t *testing.T) {
	primary := &countingChatModel{err: errors.New("primary unavailable")}
	fallback := &countingChatModel{err: errors.New("fallback unavailable")}
	guarded := guardChatModel(primary, nil, "fake", "primary", GuardrailConfig{FallbackChatModel: fallback})

	_, err := guarded.Generate(context.Background(), []*schema.Message{schema.UserMessage("prompt")})
	if err == nil || !strings.Contains(err.Error(), "fallback unavailable") {
		t.Fatalf("Generate() error = %v, want fallback error", err)
	}
	if primary.calls() != 1 || fallback.calls() != 1 {
		t.Fatalf("calls primary=%d fallback=%d, want 1/1", primary.calls(), fallback.calls())
	}
}

func TestGuardedChatModelCircuitBreakerShortCircuit(t *testing.T) {
	recorder := &fakeAgentTraceRecorder{}
	primary := &countingChatModel{err: errors.New("primary unavailable")}
	guarded := guardChatModel(primary, recorder, "fake", "primary", GuardrailConfig{
		FailureThreshold:    1,
		CircuitOpenCooldown: time.Minute,
	})

	_, firstErr := guarded.Generate(withModelTrace(context.Background(), "functional", "initial"), []*schema.Message{schema.UserMessage("prompt")})
	if firstErr == nil {
		t.Fatal("first Generate() expected primary failure")
	}
	_, secondErr := guarded.Generate(withModelTrace(context.Background(), "functional", "retry"), []*schema.Message{schema.UserMessage("prompt")})
	if !errors.Is(secondErr, ErrModelCircuitOpen) {
		t.Fatalf("second Generate() error = %v, want circuit open", secondErr)
	}
	if primary.calls() != 1 {
		t.Fatalf("primary calls = %d, want only first call", primary.calls())
	}
	if len(recorder.modelCalls) != 1 || recorder.modelCalls[0].Metadata["guardrail_event"] != GuardrailEventCircuitOpen {
		t.Fatalf("guardrail model calls = %#v", recorder.modelCalls)
	}
}

func joinedPrompt(messages []*schema.Message) string {
	var builder strings.Builder
	for _, message := range messages {
		if message != nil {
			builder.WriteString(message.Content)
		}
	}
	return builder.String()
}

func testSectionJSON(section string, title string) string {
	return fmt.Sprintf(`[{"section":%q,"cases":[{"title":%q,"priority_id":3,"custom_preconds":"ready","custom_steps_separated":[{"content":"do","expected":"done"}]}]}]`, section, title)
}

type countingChatModel struct {
	mu      sync.Mutex
	content string
	err     error
	count   int
}

func (m *countingChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	m.mu.Lock()
	m.count++
	m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	return schema.AssistantMessage(m.content, nil), nil
}

func (m *countingChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("stream not implemented")
}

func (m *countingChatModel) calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.count
}

type blockingChatModel struct{}

func (m *blockingChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *blockingChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
