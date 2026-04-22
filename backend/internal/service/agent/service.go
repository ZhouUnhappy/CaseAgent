package agent

import (
	"context"
	"fmt"
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

// GenerateCases generates test cases using DeepAgent coordination
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

	sections := make([]string, 0, len(generators))
	for _, generator := range generators {
		output, err := generator.run(ctx, requirements, knowledge)
		if err != nil {
			return s.deepAgent.GenerateCases(ctx, requirements, knowledge)
		}

		payload := extractJSONArrayPayload(output)
		if payload == "" {
			continue
		}
		sections = append(sections, payload)
	}

	if len(sections) == 0 {
		return s.deepAgent.GenerateCases(ctx, requirements, knowledge)
	}

	return "[" + strings.Join(sections, ",") + "]", nil
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
