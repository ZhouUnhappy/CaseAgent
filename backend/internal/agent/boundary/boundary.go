package boundary

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

// GenerateBoundaryCases generates boundary test cases
func (a *Agent) GenerateBoundaryCases(ctx context.Context, requirements string, knowledge string) (string, error) {
	prompt := fmt.Sprintf(`你是一个边界测试专家。根据以下需求和相关知识，生成边界测试用例。

需求:
%s

相关知识:
%s

请生成 JSON 格式的测试用例，重点关注:
- 参数边界值
- 边缘情况
- 无效输入

只返回 JSON，不要其他内容。`, requirements, knowledge)

	messages := []*schema.Message{
		schema.UserMessage(prompt),
	}

	result, err := a.chatModel.Generate(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("failed to generate boundary test cases: %w", err)
	}

	return result.Content, nil
}

func (a *Agent) GetType() string {
	return "BoundaryAgent"
}
