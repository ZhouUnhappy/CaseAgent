package agent

import (
	"context"
	"fmt"

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
		switch appCfg.Model.Chat.Provider {
		case "ark":
			chatModel, err = ark.NewChatModel(ctx, &ark.ChatModelConfig{
				APIKey:  appCfg.Model.Chat.APIKey,
				BaseURL: appCfg.Model.Chat.BaseURL,
				Model:   appCfg.Model.Chat.Model,
			})
		default:
			return nil, fmt.Errorf("unsupported chat model provider: %s", appCfg.Model.Chat.Provider)
		}

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
	return s.deepAgent.GenerateCases(ctx, requirements, knowledge)
}
