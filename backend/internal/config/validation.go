package config

import (
	"fmt"
	"strings"
)

var supportedChatProviders = map[string]struct{}{
	"ark":      {},
	"deepseek": {},
	"openai":   {},
	"fake":     {},
}

var supportedEmbeddingProviders = map[string]struct{}{
	"ark":    {},
	"openai": {},
	"fake":   {},
}

var supportedFakeScenarios = map[string]struct{}{
	"":                {},
	"valid_json":      {},
	"invalid_json":    {},
	"empty_array":     {},
	"timeout":         {},
	"rate_limit":      {},
	"partial_failure": {},
}

var supportedJobTypes = map[string]struct{}{
	"analyze":             {},
	"generate":            {},
	"document_process":    {},
	"document_reprocess":  {},
	"knowledge_process":   {},
	"knowledge_reprocess": {},
}

func Validate(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config validation: config is nil")
	}
	if err := validateDatabase(cfg.Database); err != nil {
		return err
	}
	if err := validateChat(cfg.Model.Chat); err != nil {
		return err
	}
	if err := validateEmbedding(cfg.Model.Embedding); err != nil {
		return err
	}
	if err := validateJobRunner(cfg.JobRunner); err != nil {
		return err
	}
	return nil
}

func validateDatabase(cfg DatabaseConfig) error {
	if strings.TrimSpace(cfg.Host) == "" {
		return fmt.Errorf("config validation: database.host is required")
	}
	if cfg.Port <= 0 {
		return fmt.Errorf("config validation: database.port must be > 0")
	}
	if strings.TrimSpace(cfg.User) == "" {
		return fmt.Errorf("config validation: database.user is required")
	}
	if strings.TrimSpace(cfg.DBName) == "" {
		return fmt.Errorf("config validation: database.dbname is required")
	}
	if strings.TrimSpace(cfg.SSLMode) == "" {
		return fmt.Errorf("config validation: database.sslmode is required")
	}
	return nil
}

func validateChat(cfg ChatModelConfig) error {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if _, ok := supportedChatProviders[provider]; !ok {
		return fmt.Errorf("config validation: unsupported model.chat.provider %q", cfg.Provider)
	}
	if provider == "fake" {
		if _, ok := supportedFakeScenarios[strings.TrimSpace(cfg.Model)]; !ok {
			return fmt.Errorf("config validation: unsupported fake chat scenario %q", cfg.Model)
		}
		return nil
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return fmt.Errorf("config validation: model.chat.model is required")
	}
	switch provider {
	case "ark":
		if strings.TrimSpace(cfg.BaseURL) == "" {
			return fmt.Errorf("config validation: model.chat.base_url is required for ark")
		}
		if !hasAPIKeyOrAccessPair(cfg.APIKey, cfg.AccessKey, cfg.SecretKey) {
			return fmt.Errorf("config validation: model.chat requires api_key or access_key/secret_key")
		}
	case "deepseek":
		if strings.TrimSpace(cfg.APIKey) == "" {
			return fmt.Errorf("config validation: model.chat.api_key is required for deepseek")
		}
	case "openai":
		if strings.TrimSpace(cfg.BaseURL) == "" {
			return fmt.Errorf("config validation: model.chat.base_url is required for openai")
		}
		if strings.TrimSpace(cfg.APIKey) == "" {
			return fmt.Errorf("config validation: model.chat.api_key is required for openai")
		}
	}
	return nil
}

func validateEmbedding(cfg EmbeddingModelConfig) error {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if _, ok := supportedEmbeddingProviders[provider]; !ok {
		return fmt.Errorf("config validation: unsupported model.embedding.provider %q", cfg.Provider)
	}
	if cfg.Dimensions <= 0 {
		return fmt.Errorf("config validation: model.embedding.dimensions must be > 0")
	}
	if provider == "fake" {
		return nil
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return fmt.Errorf("config validation: model.embedding.model is required")
	}
	switch provider {
	case "ark":
		if strings.TrimSpace(cfg.BaseURL) == "" {
			return fmt.Errorf("config validation: model.embedding.base_url is required for ark")
		}
		if !hasAPIKeyOrAccessPair(cfg.APIKey, cfg.AccessKey, cfg.SecretKey) {
			return fmt.Errorf("config validation: model.embedding requires api_key or access_key/secret_key")
		}
	case "openai":
		if strings.TrimSpace(cfg.BaseURL) == "" {
			return fmt.Errorf("config validation: model.embedding.base_url is required for openai")
		}
		if strings.TrimSpace(cfg.APIKey) == "" {
			return fmt.Errorf("config validation: model.embedding.api_key is required for openai")
		}
	}
	return nil
}

func validateJobRunner(cfg JobRunnerConfig) error {
	if cfg.MaxConcurrency <= 0 {
		return fmt.Errorf("config validation: job_runner.max_concurrency must be > 0")
	}
	if cfg.MaxRetries < 0 {
		return fmt.Errorf("config validation: job_runner.max_retries must be >= 0")
	}
	if cfg.RetryBackoffSeconds < 0 {
		return fmt.Errorf("config validation: job_runner.retry_backoff_seconds must be >= 0")
	}
	if cfg.PollIntervalSeconds <= 0 {
		return fmt.Errorf("config validation: job_runner.poll_interval_seconds must be > 0")
	}
	if cfg.RunningTimeoutSeconds <= 0 {
		return fmt.Errorf("config validation: job_runner.running_timeout_seconds must be > 0")
	}
	if cfg.StateUpdateTimeoutSeconds <= 0 {
		return fmt.Errorf("config validation: job_runner.state_update_timeout_seconds must be > 0")
	}
	for jobType, options := range cfg.Types {
		if _, ok := supportedJobTypes[jobType]; !ok {
			return fmt.Errorf("config validation: unsupported job_runner.types key %q", jobType)
		}
		if options.MaxConcurrency < 0 {
			return fmt.Errorf("config validation: job_runner.types.%s.max_concurrency must be >= 0", jobType)
		}
		if options.MaxRetries < 0 {
			return fmt.Errorf("config validation: job_runner.types.%s.max_retries must be >= 0", jobType)
		}
	}
	return nil
}

func hasAPIKeyOrAccessPair(apiKey string, accessKey string, secretKey string) bool {
	if strings.TrimSpace(apiKey) != "" {
		return true
	}
	return strings.TrimSpace(accessKey) != "" && strings.TrimSpace(secretKey) != ""
}
