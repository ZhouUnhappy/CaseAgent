package deep

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type Agent struct {
	chatModel model.BaseChatModel
	subAgents []adk.Agent
}

type Config struct {
	ChatModel model.BaseChatModel
	SubAgents []adk.Agent
}

func New(ctx context.Context, cfg *Config) (*Agent, error) {
	if cfg.ChatModel == nil {
		return nil, fmt.Errorf("chat model is required")
	}

	return &Agent{
		chatModel: cfg.ChatModel,
		subAgents: cfg.SubAgents,
	}, nil
}

// GenerateCases generates test cases by coordinating sub-agents
func (a *Agent) GenerateCases(ctx context.Context, requirements string, knowledge string) (string, error) {
	// For now, use the chat model directly to generate comprehensive test cases
	// TODO: Implement full coordination with sub-agents
	prompt := fmt.Sprintf(`你是一个测试用例生成专家。根据以下需求和相关知识，生成全面的测试用例。

需求:
%s

相关知识:
%s

请生成 JSON 格式的测试用例数组，包含以下类型的测试:
1. 功能测试
2. 运维测试
3. 故障测试
4. 边界测试

每个测试用例包含:
- type: 测试类型
- title: 测试用例标题
- description: 测试用例描述
- steps: 测试步骤数组
- expected_result: 预期结果

只返回 JSON 数组，不要其他内容。`, requirements, knowledge)

	messages := []*schema.Message{
		schema.UserMessage(prompt),
	}

	result, err := a.chatModel.Generate(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("failed to generate test cases: %w", err)
	}

	return result.Content, nil
}

func (a *Agent) GetType() string {
	return "DeepAgent"
}
