package generation

import (
	"testing"

	"caseagent/internal/config"
)

func TestFromConfigBuildsStableProfile(t *testing.T) {
	cfg := &config.Config{
		Model: config.ModelConfig{
			Chat: config.ChatModelConfig{
				Provider:                       "OpenAI",
				Model:                          "gpt-5",
				RequestTimeoutSeconds:          45,
				ProviderTimeoutSeconds:         20,
				TaskBudgetTokens:               12000,
				CircuitBreakerFailureThreshold: 3,
				CircuitBreakerCooldownSeconds:  90,
				Fallback: config.ChatFallbackConfig{
					Provider:               "deepseek",
					Model:                  "deepseek-v4",
					ProviderTimeoutSeconds: 10,
				},
			},
		},
	}
	versions := map[string]string{
		"agent.functional.cases": "v1",
		"agent.deep.cases":       "v2",
	}

	profile := FromConfig(cfg, versions)
	if profile.ID != ProfileID || profile.Version == "" {
		t.Fatalf("unexpected profile id/version: %#v", profile)
	}
	if profile.Provider != "openai" || profile.Model != "gpt-5" {
		t.Fatalf("unexpected provider/model: %#v", profile)
	}
	if !profile.FallbackEnabled || profile.FallbackProvider != "deepseek" || profile.FallbackModel != "deepseek-v4" {
		t.Fatalf("unexpected fallback profile: %#v", profile)
	}
	if profile.DocumentTopK != 5 || profile.KnowledgeTopK != 5 ||
		profile.DocumentQueryFragments != 4 || profile.KnowledgeQueryFragments != 3 ||
		profile.SourceContextChunkPreview != 3 {
		t.Fatalf("unexpected retrieval profile constants: %#v", profile)
	}

	again := FromConfig(cfg, map[string]string{
		"agent.deep.cases":       "v2",
		"agent.functional.cases": "v1",
	})
	if again.Version != profile.Version {
		t.Fatalf("version should be stable across prompt map order: %s vs %s", again.Version, profile.Version)
	}
}

func TestFromConfigVersionChangesWithPromptVersion(t *testing.T) {
	cfg := &config.Config{Model: config.ModelConfig{Chat: config.ChatModelConfig{Provider: "fake", Model: "valid_json"}}}
	v1 := FromConfig(cfg, map[string]string{"agent.functional.cases": "v1"})
	v2 := FromConfig(cfg, map[string]string{"agent.functional.cases": "v2"})
	if v1.Version == v2.Version {
		t.Fatalf("version should change when prompt version changes: %s", v1.Version)
	}
}

func TestFromConfigDefaultsMissingConfig(t *testing.T) {
	profile := FromConfig(nil, nil)
	if profile.Provider != "unknown" || profile.Model != "default" {
		t.Fatalf("unexpected nil config defaults: %#v", profile)
	}
	if profile.RequestTimeoutSeconds != 60 || profile.CircuitBreakerCooldownSeconds != 60 {
		t.Fatalf("unexpected timeout defaults: %#v", profile)
	}
}
