package functional

import (
	"context"
	"fmt"

	"caseagent/internal/agent/prompts"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type Agent struct {
	chatModel model.BaseChatModel
	prompts   *prompts.Registry
}

type Config struct {
	ChatModel model.BaseChatModel
	Prompts   *prompts.Registry
}

func New(ctx context.Context, cfg *Config) (*Agent, error) {
	if cfg.ChatModel == nil {
		return nil, fmt.Errorf("chat model is required")
	}
	registry := cfg.Prompts
	if registry == nil {
		registry = prompts.DefaultRegistry()
	}

	return &Agent{
		chatModel: cfg.ChatModel,
		prompts:   registry,
	}, nil
}

// GenerateFunctionalCases generates functional test cases
func (a *Agent) GenerateFunctionalCases(ctx context.Context, requirements string, knowledge string) (string, error) {
	rendered, err := a.prompts.Render(prompts.FunctionalCases, prompts.CasePromptData{
		Requirements: requirements,
		Knowledge:    knowledge,
	})
	if err != nil {
		return "", err
	}

	messages := []*schema.Message{
		schema.UserMessage(rendered.Content),
	}

	result, err := a.chatModel.Generate(prompts.WithRenderedPrompt(ctx, rendered), messages)
	if err != nil {
		return "", fmt.Errorf("failed to generate functional test cases: %w", err)
	}

	return result.Content, nil
}

func (a *Agent) GetType() string {
	return "FunctionalAgent"
}
