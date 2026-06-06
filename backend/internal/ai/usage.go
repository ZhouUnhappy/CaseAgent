package ai

import "github.com/cloudwego/eino/schema"

type UsageEstimate struct {
	PromptChars               int    `json:"prompt_chars"`
	CompletionChars           int    `json:"completion_chars"`
	TotalChars                int    `json:"total_chars"`
	EstimatedPromptTokens     int    `json:"estimated_prompt_tokens"`
	EstimatedCompletionTokens int    `json:"estimated_completion_tokens"`
	EstimatedTotalTokens      int    `json:"estimated_total_tokens"`
	PromptTokens              int    `json:"prompt_tokens,omitempty"`
	CompletionTokens          int    `json:"completion_tokens,omitempty"`
	TotalTokens               int    `json:"total_tokens,omitempty"`
	TokenSource               string `json:"token_source"`
}

func EstimateMessageUsage(input []*schema.Message, output *schema.Message) UsageEstimate {
	promptChars := MessageChars(input)
	completionChars := MessageChars([]*schema.Message{output})
	return EstimateUsage(promptChars, completionChars, responseUsage(output))
}

func EstimateUsage(promptChars int, completionChars int, usage *schema.TokenUsage) UsageEstimate {
	estimate := UsageEstimate{
		PromptChars:               nonNegative(promptChars),
		CompletionChars:           nonNegative(completionChars),
		EstimatedPromptTokens:     EstimateTokensFromChars(promptChars),
		EstimatedCompletionTokens: EstimateTokensFromChars(completionChars),
		TokenSource:               "estimated_chars",
	}
	estimate.TotalChars = estimate.PromptChars + estimate.CompletionChars
	estimate.EstimatedTotalTokens = estimate.EstimatedPromptTokens + estimate.EstimatedCompletionTokens

	if usage != nil && usage.TotalTokens > 0 {
		estimate.PromptTokens = usage.PromptTokens
		estimate.CompletionTokens = usage.CompletionTokens
		estimate.TotalTokens = usage.TotalTokens
		estimate.TokenSource = "provider_usage"
	}
	return estimate
}

func EstimatePromptTokens(messages []*schema.Message) int {
	return EstimateTokensFromChars(MessageChars(messages))
}

func EstimateTokensFromChars(chars int) int {
	if chars <= 0 {
		return 0
	}
	return (chars + 3) / 4
}

func MessageChars(messages []*schema.Message) int {
	total := 0
	for _, message := range messages {
		if message == nil {
			continue
		}
		total += len(message.Content)
		total += len(message.ReasoningContent)
		for _, part := range message.MultiContent {
			total += len(part.Text)
		}
		for _, part := range message.UserInputMultiContent {
			total += len(part.Text)
		}
		for _, part := range message.AssistantGenMultiContent {
			total += len(part.Text)
		}
	}
	return total
}

func AccountedTokens(usage UsageEstimate) int {
	if usage.TotalTokens > 0 {
		return usage.TotalTokens
	}
	return usage.EstimatedTotalTokens
}

func UsageMetadata(usage UsageEstimate) map[string]any {
	return map[string]any{
		"prompt_chars":                usage.PromptChars,
		"completion_chars":            usage.CompletionChars,
		"total_chars":                 usage.TotalChars,
		"estimated_prompt_tokens":     usage.EstimatedPromptTokens,
		"estimated_completion_tokens": usage.EstimatedCompletionTokens,
		"estimated_total_tokens":      usage.EstimatedTotalTokens,
		"prompt_tokens":               usage.PromptTokens,
		"completion_tokens":           usage.CompletionTokens,
		"total_tokens":                usage.TotalTokens,
		"token_source":                usage.TokenSource,
		"accounted_tokens":            AccountedTokens(usage),
	}
}

func responseUsage(message *schema.Message) *schema.TokenUsage {
	if message == nil || message.ResponseMeta == nil {
		return nil
	}
	return message.ResponseMeta.Usage
}

func nonNegative(value int) int {
	if value < 0 {
		return 0
	}
	return value
}
