package failure

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

// GenerateFailureCases generates failure test cases
func (a *Agent) GenerateFailureCases(ctx context.Context, requirements string, knowledge string) (string, error) {
	prompt := fmt.Sprintf(`你是一个故障测试专家。根据以下需求和相关知识，生成故障测试用例。

需求:
%s

相关知识:
%s

请生成 JSON 格式的测试用例，重点关注:
- 节点重启
- 断电恢复
- 网络分区

只返回 JSON，不要其他内容。`, requirements, knowledge)

	messages := []*schema.Message{
		schema.UserMessage(prompt),
	}

	result, err := a.chatModel.Generate(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("failed to generate failure test cases: %w", err)
	}

	return result.Content, nil
}

func (a *Agent) GetType() string {
	return "FailureAgent"
}
