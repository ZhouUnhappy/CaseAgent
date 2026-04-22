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

// GenerateCases generates sectioned test cases by coordinating sub-agents.
func (a *Agent) GenerateCases(ctx context.Context, requirements string, knowledge string) (string, error) {
	// For now, use the chat model directly to generate comprehensive test cases.
	// TODO: Implement full coordination with sub-agents.
	prompt := fmt.Sprintf(`你是一个测试用例生成专家。根据以下需求和相关知识，输出结构化测试用例。

需求:
%s

相关知识:
%s

请生成 JSON 数组，每个元素表示一个 section，结构必须严格如下：
[
  {
    "section": "功能测试",
    "cases": [
      {
        "title": "[模块] 用例标题",
        "priority_id": 3,
        "custom_preconds": "前置条件",
        "custom_steps_separated": [
          {"content": "步骤1", "expected": "预期1"},
          {"content": "步骤2", "expected": "预期2"}
        ]
      }
    ]
  }
]

要求：
1. 功能测试
2. 运维测试
3. 故障测试
4. 边界测试
5. priority_id 取值 1-4，默认高优先级可用 3
6. custom_steps_separated 中每一步都必须包含 content 和 expected
7. 只返回合法 JSON，不要 Markdown 代码块，不要解释文字`, requirements, knowledge)

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
