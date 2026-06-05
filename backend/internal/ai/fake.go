package ai

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const (
	fakeScenarioValid          = "valid_json"
	fakeScenarioInvalidJSON    = "invalid_json"
	fakeScenarioEmptyArray     = "empty_array"
	fakeScenarioTimeout        = "timeout"
	fakeScenarioRateLimit      = "rate_limit"
	fakeScenarioPartialFailure = "partial_failure"
)

type fakeChatModel struct {
	scenario string
}

func newFakeChatModel(scenario string) model.BaseChatModel {
	scenario = strings.TrimSpace(scenario)
	if scenario == "" {
		scenario = fakeScenarioValid
	}
	return &fakeChatModel{scenario: scenario}
}

func (m *fakeChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	switch m.scenario {
	case fakeScenarioTimeout:
		<-ctx.Done()
		return nil, ctx.Err()
	case fakeScenarioRateLimit:
		return nil, fmt.Errorf("fake chat rate limited")
	case fakeScenarioInvalidJSON:
		return schema.AssistantMessage("{not-json", nil), nil
	case fakeScenarioEmptyArray:
		return schema.AssistantMessage("[]", nil), nil
	case fakeScenarioPartialFailure:
		if promptContains(input, "运维测试专家") || promptContains(input, "故障测试专家") {
			return nil, fmt.Errorf("fake sub-agent partial failure")
		}
	}

	return schema.AssistantMessage(fakeJSONForPrompt(input), nil), nil
}

func (m *fakeChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, fmt.Errorf("fake chat stream is not implemented")
}

type fakeEmbedder struct {
	dimensions int
}

func newFakeEmbedder(dimensions int) embedding.Embedder {
	if dimensions <= 0 {
		dimensions = 4
	}
	return &fakeEmbedder{dimensions: dimensions}
}

func (e *fakeEmbedder) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	results := make([][]float64, 0, len(texts))
	for _, text := range texts {
		results = append(results, deterministicVector(text, e.dimensions))
	}
	return results, nil
}

func deterministicVector(text string, dimensions int) []float64 {
	vector := make([]float64, dimensions)
	for i := range vector {
		h := fnv.New32a()
		_, _ = h.Write([]byte(fmt.Sprintf("%s:%d", text, i)))
		vector[i] = float64(h.Sum32()%1000) / 1000
	}
	return vector
}

func promptContains(messages []*schema.Message, needle string) bool {
	for _, message := range messages {
		if strings.Contains(message.Content, needle) {
			return true
		}
	}
	return false
}

func fakeJSONForPrompt(messages []*schema.Message) string {
	switch {
	case promptContains(messages, "总协调 Agent"), promptContains(messages, "测试用例生成专家"):
		return `[
  {
    "section": "功能测试",
    "cases": [
      {
        "title": "[Fake] happy path",
        "priority_id": 3,
        "custom_preconds": "fake precondition",
        "custom_steps_separated": [
          {"content": "execute fake flow", "expected": "fake result is visible"}
        ]
      }
    ]
  }
]`
	case promptContains(messages, "运维测试专家"):
		return fakeSectionJSON("运维测试", "[Fake] ops path")
	case promptContains(messages, "故障测试专家"):
		return fakeSectionJSON("故障测试", "[Fake] failure path")
	case promptContains(messages, "边界测试专家"):
		return fakeSectionJSON("边界测试", "[Fake] boundary path")
	default:
		return fakeSectionJSON("功能测试", "[Fake] functional path")
	}
}

func fakeSectionJSON(section string, title string) string {
	return fmt.Sprintf(`[
  {
    "section": %q,
    "cases": [
      {
        "title": %q,
        "priority_id": 3,
        "custom_preconds": "fake precondition",
        "custom_steps_separated": [
          {"content": "execute fake flow", "expected": "fake result is visible"}
        ]
      }
    ]
  }
]`, section, title)
}
