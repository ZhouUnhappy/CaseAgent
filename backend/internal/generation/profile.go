package generation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"caseagent/internal/agent/prompts"
	"caseagent/internal/config"
)

const (
	ProfileID                 = "caseagent-generation-default"
	DocumentTopK              = 5
	KnowledgeTopK             = 5
	DocumentQueryFragments    = 4
	KnowledgeQueryFragments   = 3
	SourceContextChunkPreview = 3
)

type Profile struct {
	ID                             string            `json:"id"`
	Version                        string            `json:"version"`
	Provider                       string            `json:"provider"`
	Model                          string            `json:"model"`
	PromptVersions                 map[string]string `json:"prompt_versions"`
	DocumentTopK                   int               `json:"document_top_k"`
	KnowledgeTopK                  int               `json:"knowledge_top_k"`
	DocumentQueryFragments         int               `json:"document_query_fragments"`
	KnowledgeQueryFragments        int               `json:"knowledge_query_fragments"`
	SourceContextChunkPreview      int               `json:"source_context_chunk_preview"`
	RequestTimeoutSeconds          int               `json:"request_timeout_seconds"`
	ProviderTimeoutSeconds         int               `json:"provider_timeout_seconds"`
	TaskBudgetTokens               int               `json:"task_budget_tokens"`
	CircuitBreakerFailureThreshold int               `json:"circuit_breaker_failure_threshold"`
	CircuitBreakerCooldownSeconds  int               `json:"circuit_breaker_cooldown_seconds"`
	FallbackEnabled                bool              `json:"fallback_enabled"`
	FallbackProvider               string            `json:"fallback_provider,omitempty"`
	FallbackModel                  string            `json:"fallback_model,omitempty"`
	FallbackProviderTimeoutSeconds int               `json:"fallback_provider_timeout_seconds,omitempty"`
}

func CurrentProfile() Profile {
	return FromConfig(config.Get(), promptVersions(prompts.DefaultRegistry()))
}

func FromConfig(cfg *config.Config, versions map[string]string) Profile {
	chat := config.ChatModelConfig{}
	if cfg != nil {
		chat = cfg.Model.Chat
	}
	profile := Profile{
		ID:                             ProfileID,
		Provider:                       normalizeProvider(chat.Provider),
		Model:                          nonEmpty(chat.Model, "default"),
		PromptVersions:                 normalizePromptVersions(versions),
		DocumentTopK:                   DocumentTopK,
		KnowledgeTopK:                  KnowledgeTopK,
		DocumentQueryFragments:         DocumentQueryFragments,
		KnowledgeQueryFragments:        KnowledgeQueryFragments,
		SourceContextChunkPreview:      SourceContextChunkPreview,
		RequestTimeoutSeconds:          defaultPositive(chat.RequestTimeoutSeconds, 60),
		ProviderTimeoutSeconds:         chat.ProviderTimeoutSeconds,
		TaskBudgetTokens:               chat.TaskBudgetTokens,
		CircuitBreakerFailureThreshold: chat.CircuitBreakerFailureThreshold,
		CircuitBreakerCooldownSeconds:  defaultPositive(chat.CircuitBreakerCooldownSeconds, 60),
		FallbackProvider:               normalizeProvider(chat.Fallback.Provider),
		FallbackModel:                  strings.TrimSpace(chat.Fallback.Model),
		FallbackProviderTimeoutSeconds: chat.Fallback.ProviderTimeoutSeconds,
	}
	profile.FallbackEnabled = profile.FallbackProvider != "" || profile.FallbackModel != ""
	profile.Version = profile.fingerprint()
	return profile
}

func promptVersions(registry *prompts.Registry) map[string]string {
	defaults := registry.DefaultVersions()
	versions := make(map[string]string, len(defaults))
	for id, version := range defaults {
		versions[string(id)] = version
	}
	return versions
}

func (p Profile) fingerprint() string {
	parts := []string{
		p.ID,
		p.Provider,
		p.Model,
		fmt.Sprintf("doc_top_k=%d", p.DocumentTopK),
		fmt.Sprintf("knowledge_top_k=%d", p.KnowledgeTopK),
		fmt.Sprintf("doc_query_fragments=%d", p.DocumentQueryFragments),
		fmt.Sprintf("knowledge_query_fragments=%d", p.KnowledgeQueryFragments),
		fmt.Sprintf("chunk_preview=%d", p.SourceContextChunkPreview),
		fmt.Sprintf("request_timeout=%d", p.RequestTimeoutSeconds),
		fmt.Sprintf("provider_timeout=%d", p.ProviderTimeoutSeconds),
		fmt.Sprintf("budget=%d", p.TaskBudgetTokens),
		fmt.Sprintf("circuit_threshold=%d", p.CircuitBreakerFailureThreshold),
		fmt.Sprintf("circuit_cooldown=%d", p.CircuitBreakerCooldownSeconds),
		fmt.Sprintf("fallback_enabled=%t", p.FallbackEnabled),
		p.FallbackProvider,
		p.FallbackModel,
		fmt.Sprintf("fallback_timeout=%d", p.FallbackProviderTimeoutSeconds),
	}
	for _, key := range sortedKeys(p.PromptVersions) {
		parts = append(parts, "prompt:"+key+"="+p.PromptVersions[key])
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return "v1-" + hex.EncodeToString(sum[:])[:12]
}

func normalizePromptVersions(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func normalizeProvider(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}
	return value
}

func nonEmpty(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func defaultPositive(value int, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}
