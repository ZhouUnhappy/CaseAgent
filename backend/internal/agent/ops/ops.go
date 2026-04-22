package ops

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type Agent struct {
	chatModel model.BaseChatModel
}

type Config struct {
	ChatModel model.BaseChatModel
}

func New(ctx context.Context, cfg *Config) (*Agent, error) {
	if cfg.ChatModel == nil {
		return nil, fmt.Errorf("chat model is required")
	}

	return &Agent{
		chatModel: cfg.ChatModel,
	}, nil
}

// GenerateOpsCases generates operations test cases
func (a *Agent) GenerateOpsCases(ctx context.Context, requirements string, knowledge string) (string, error) {
	prompt := fmt.Sprintf(`你是一个运维测试专家。根据以下需求和相关知识，只生成运维测试用例。

需求:
%s

相关知识:
%s

请重点关注：
- 升级场景
- 扩容场景
- 维护场景

请只返回如下结构的 JSON 数组，不要解释文字，不要 Markdown 代码块：
[
  {
    "section": "运维测试",
    "cases": [
      {
        "title": "[模块] 用例标题",
        "priority_id": 3,
        "custom_preconds": "前置条件",
        "custom_steps_separated": [
          {"content": "步骤1", "expected": "预期1"}
        ]
      }
    ]
  }
]`, requirements, knowledge)

	messages := []*schema.Message{
		schema.UserMessage(prompt),
	}

	result, err := a.chatModel.Generate(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("failed to generate ops test cases: %w", err)
	}

	return result.Content, nil
}

func (a *Agent) GetType() string {
	return "OpsAgent"
}
