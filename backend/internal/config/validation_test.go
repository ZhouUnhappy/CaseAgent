package config

import (
	"strings"
	"testing"
)

func TestValidateAcceptsFakeProviders(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.Model.Chat.Provider = "fake"
	cfg.Model.Chat.Model = "partial_failure"
	cfg.Model.Chat.APIKey = ""
	cfg.Model.Chat.BaseURL = ""
	cfg.Model.Embedding.Provider = "fake"
	cfg.Model.Embedding.Model = ""
	cfg.Model.Embedding.APIKey = ""
	cfg.Model.Embedding.BaseURL = ""

	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate() returned error: %v", err)
	}
}

func TestValidateRejectsMissingChatAuth(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.Model.Chat.APIKey = ""

	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "model.chat requires api_key") {
		t.Fatalf("Validate() error = %v, want chat auth error", err)
	}
}

func TestValidateRejectsMissingEmbeddingDimensions(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.Model.Embedding.Dimensions = 0

	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "model.embedding.dimensions") {
		t.Fatalf("Validate() error = %v, want dimensions error", err)
	}
}

func TestValidateRejectsInvalidJobRunnerRange(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.JobRunner.PollIntervalSeconds = 0

	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "poll_interval_seconds") {
		t.Fatalf("Validate() error = %v, want poll interval error", err)
	}
}

func TestValidateRejectsUnknownJobTypeOverride(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.JobRunner.Types = map[string]JobTypeRunnerConfig{
		"unknown": {MaxConcurrency: 1},
	}

	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "unsupported job_runner.types key") {
		t.Fatalf("Validate() error = %v, want job type error", err)
	}
}

func minimalValidConfig() *Config {
	return &Config{
		Server: ServerConfig{Mode: "debug", Port: 40003},
		Database: DatabaseConfig{
			Host:    "localhost",
			Port:    5432,
			User:    "caseagent",
			DBName:  "caseagent",
			SSLMode: "disable",
		},
		Model: ModelConfig{
			Chat: ChatModelConfig{
				Provider: "ark",
				Model:    "chat-model",
				APIKey:   "test-key",
				BaseURL:  "https://example.test/api/v3",
			},
			Embedding: EmbeddingModelConfig{
				Provider:   "ark",
				Model:      "embedding-model",
				Dimensions: 6,
				APIKey:     "test-key",
				BaseURL:    "https://example.test/api/v3",
			},
		},
		JobRunner: JobRunnerConfig{
			MaxConcurrency:            2,
			MaxRetries:                2,
			RetryBackoffSeconds:       5,
			PollIntervalSeconds:       2,
			RunningTimeoutSeconds:     900,
			StateUpdateTimeoutSeconds: 10,
		},
	}
}
