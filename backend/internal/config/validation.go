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
	if err := validateRetention(cfg.Retention); err != nil {
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
	if err := validateChatGuardrails("model.chat", cfg.RequestTimeoutSeconds, cfg.ProviderTimeoutSeconds, cfg.TaskBudgetTokens, cfg.CircuitBreakerFailureThreshold, cfg.CircuitBreakerCooldownSeconds); err != nil {
		return err
	}
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if _, ok := supportedChatProviders[provider]; !ok {
		return fmt.Errorf("config validation: unsupported model.chat.provider %q", cfg.Provider)
	}
	if err := validateChatProvider("model.chat", provider, cfg.Model, cfg.APIKey, cfg.BaseURL); err != nil {
		return err
	}
	if err := validateChatFallback(cfg.Fallback); err != nil {
		return err
	}
	return nil
}

func validateChatProvider(path string, provider string, model string, apiKey string, baseURL string) error {
	if provider == "fake" {
		if _, ok := supportedFakeScenarios[strings.TrimSpace(model)]; !ok {
			return fmt.Errorf("config validation: unsupported fake chat scenario %q", model)
		}
		return nil
	}
	if strings.TrimSpace(model) == "" {
		return fmt.Errorf("config validation: %s.model is required", path)
	}
	switch provider {
	case "ark":
		if strings.TrimSpace(baseURL) == "" {
			return fmt.Errorf("config validation: %s.base_url is required for ark", path)
		}
		if strings.TrimSpace(apiKey) == "" {
			return fmt.Errorf("config validation: %s.api_key is required for ark", path)
		}
	case "deepseek":
		if strings.TrimSpace(apiKey) == "" {
			return fmt.Errorf("config validation: %s.api_key is required for deepseek", path)
		}
	case "openai":
		if strings.TrimSpace(baseURL) == "" {
			return fmt.Errorf("config validation: %s.base_url is required for openai", path)
		}
		if strings.TrimSpace(apiKey) == "" {
			return fmt.Errorf("config validation: %s.api_key is required for openai", path)
		}
	}
	return nil
}

func validateChatGuardrails(path string, requestTimeoutSeconds int, providerTimeoutSeconds int, taskBudgetTokens int, failureThreshold int, cooldownSeconds int) error {
	if requestTimeoutSeconds < 0 {
		return fmt.Errorf("config validation: %s.request_timeout_seconds must be >= 0", path)
	}
	if providerTimeoutSeconds < 0 {
		return fmt.Errorf("config validation: %s.provider_timeout_seconds must be >= 0", path)
	}
	if taskBudgetTokens < 0 {
		return fmt.Errorf("config validation: %s.task_budget_tokens must be >= 0", path)
	}
	if failureThreshold < 0 {
		return fmt.Errorf("config validation: %s.circuit_breaker_failure_threshold must be >= 0", path)
	}
	if cooldownSeconds < 0 {
		return fmt.Errorf("config validation: %s.circuit_breaker_cooldown_seconds must be >= 0", path)
	}
	return nil
}

func validateChatFallback(cfg ChatFallbackConfig) error {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		return nil
	}
	if _, ok := supportedChatProviders[provider]; !ok {
		return fmt.Errorf("config validation: unsupported model.chat.fallback.provider %q", cfg.Provider)
	}
	if cfg.ProviderTimeoutSeconds < 0 {
		return fmt.Errorf("config validation: model.chat.fallback.provider_timeout_seconds must be >= 0")
	}
	return validateChatProvider("model.chat.fallback", provider, cfg.Model, cfg.APIKey, cfg.BaseURL)
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
		if strings.TrimSpace(cfg.APIKey) == "" {
			return fmt.Errorf("config validation: model.embedding.api_key is required for ark")
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

func validateRetention(cfg RetentionConfig) error {
	if cfg.TraceRetentionDays <= 0 {
		return fmt.Errorf("config validation: retention.trace_retention_days must be > 0")
	}
	return nil
}

func validateJobRunner(cfg JobRunnerConfig) error {
	if cfg.MaxConcurrency <= 0 {
		return fmt.Errorf("config validation: job_runner.max_concurrency must be > 0")
	}
	if cfg.TenantMaxConcurrency < 0 {
		return fmt.Errorf("config validation: job_runner.tenant_max_concurrency must be >= 0")
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
