package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type DraftInput struct {
	CandidateType  string
	CandidateName  string
	SourceSnippets []map[string]any
}

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

	return &Agent{chatModel: cfg.ChatModel}, nil
}

func (a *Agent) GenerateDraft(ctx context.Context, input DraftInput) (string, error) {
	prompt, err := BuildDraftPrompt(input)
	if err != nil {
		return "", err
	}

	result, err := a.chatModel.Generate(ctx, []*schema.Message{schema.UserMessage(prompt)})
	if err != nil {
		return "", fmt.Errorf("failed to generate knowledge draft: %w", err)
	}
	return stripMarkdownFence(result.Content), nil
}

func BuildDraftPrompt(input DraftInput) (string, error) {
	name := strings.TrimSpace(input.CandidateName)
	if name == "" {
		return "", fmt.Errorf("candidate name is required")
	}

	snippets, err := json.MarshalIndent(input.SourceSnippets, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal source snippets: %w", err)
	}

	return fmt.Sprintf(`你是一个测试平台知识库维护助手。请基于下面的知识缺口 suggestion，为知识库生成一份 Markdown 草稿。

候选类型: %s
候选名称: %s
来源片段(JSON):
%s

输出要求：
1. 只返回 Markdown 正文，不要 Markdown 代码块，不要解释前言。
2. 这是待人工校对的草稿，不要把无法从来源片段确认的信息写成确定事实；无法确认处写「待确认」。
3. 使用以下结构：
# %s

## 概述
用 2-4 句话说明该产品/模块可能承担的职责；信息不足时明确待确认。

## 相关服务/模块
列出来源片段中出现的相关对象；没有证据时写「待确认」。

## 工作原理
从需求或用例上下文推断流程，只写有依据的内容；不确定的步骤标记待确认。

## 测试关注点
列出后续生成测试用例时应关注的功能、边界、故障或运维点。

## 待人工校对
列出需要产品/研发确认的问题。`, input.CandidateType, name, string(snippets), name), nil
}

func stripMarkdownFence(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}

	lines := strings.Split(trimmed, "\n")
	if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
		lines = lines[1:]
	}
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
		lines = lines[:len(lines)-1]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
