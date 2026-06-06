package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	Database   DatabaseConfig   `mapstructure:"database"`
	Model      ModelConfig      `mapstructure:"model"`
	Suggestion SuggestionConfig `mapstructure:"suggestion"`
	JobRunner  JobRunnerConfig  `mapstructure:"job_runner"`
	GWS        GWSConfig        `mapstructure:"gws"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

type ModelConfig struct {
	Chat      ChatModelConfig      `mapstructure:"chat"`
	Embedding EmbeddingModelConfig `mapstructure:"embedding"`
}

type ChatModelConfig struct {
	Provider                       string             `mapstructure:"provider"`
	Model                          string             `mapstructure:"model"`
	APIKey                         string             `mapstructure:"api_key"`
	AccessKey                      string             `mapstructure:"access_key"`
	SecretKey                      string             `mapstructure:"secret_key"`
	BaseURL                        string             `mapstructure:"base_url"`
	Region                         string             `mapstructure:"region"`
	RequestTimeoutSeconds          int                `mapstructure:"request_timeout_seconds"`
	ProviderTimeoutSeconds         int                `mapstructure:"provider_timeout_seconds"`
	TaskBudgetTokens               int                `mapstructure:"task_budget_tokens"`
	CircuitBreakerFailureThreshold int                `mapstructure:"circuit_breaker_failure_threshold"`
	CircuitBreakerCooldownSeconds  int                `mapstructure:"circuit_breaker_cooldown_seconds"`
	Fallback                       ChatFallbackConfig `mapstructure:"fallback"`
}

type ChatFallbackConfig struct {
	Provider               string `mapstructure:"provider"`
	Model                  string `mapstructure:"model"`
	APIKey                 string `mapstructure:"api_key"`
	AccessKey              string `mapstructure:"access_key"`
	SecretKey              string `mapstructure:"secret_key"`
	BaseURL                string `mapstructure:"base_url"`
	Region                 string `mapstructure:"region"`
	ProviderTimeoutSeconds int    `mapstructure:"provider_timeout_seconds"`
}

type EmbeddingModelConfig struct {
	Provider   string `mapstructure:"provider"`
	Model      string `mapstructure:"model"`
	Dimensions int    `mapstructure:"dimensions"`
	APIKey     string `mapstructure:"api_key"`
	AccessKey  string `mapstructure:"access_key"`
	SecretKey  string `mapstructure:"secret_key"`
	BaseURL    string `mapstructure:"base_url"`
	Region     string `mapstructure:"region"`
}

type GWSConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Command string `mapstructure:"command"`
}

type SuggestionConfig struct {
	AutoDismissPendingDays int `mapstructure:"auto_dismiss_pending_days"`
}

type JobRunnerConfig struct {
	MaxConcurrency            int                            `mapstructure:"max_concurrency"`
	TenantMaxConcurrency      int                            `mapstructure:"tenant_max_concurrency"`
	MaxRetries                int                            `mapstructure:"max_retries"`
	RetryBackoffSeconds       int                            `mapstructure:"retry_backoff_seconds"`
	PollIntervalSeconds       int                            `mapstructure:"poll_interval_seconds"`
	RunningTimeoutSeconds     int                            `mapstructure:"running_timeout_seconds"`
	StateUpdateTimeoutSeconds int                            `mapstructure:"state_update_timeout_seconds"`
	Types                     map[string]JobTypeRunnerConfig `mapstructure:"types"`
}

type JobTypeRunnerConfig struct {
	MaxConcurrency int `mapstructure:"max_concurrency"`
	MaxRetries     int `mapstructure:"max_retries"`
}

var cfg *Config

func Load(configPath string) error {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.SetDefault("model.chat.request_timeout_seconds", 60)
	viper.SetDefault("model.chat.provider_timeout_seconds", 0)
	viper.SetDefault("model.chat.task_budget_tokens", 0)
	viper.SetDefault("model.chat.circuit_breaker_failure_threshold", 0)
	viper.SetDefault("model.chat.circuit_breaker_cooldown_seconds", 60)
	viper.SetDefault("suggestion.auto_dismiss_pending_days", 30)
	viper.SetDefault("job_runner.max_concurrency", 2)
	viper.SetDefault("job_runner.tenant_max_concurrency", 0)
	viper.SetDefault("job_runner.max_retries", 2)
	viper.SetDefault("job_runner.retry_backoff_seconds", 5)
	viper.SetDefault("job_runner.poll_interval_seconds", 2)
	viper.SetDefault("job_runner.running_timeout_seconds", 900)
	viper.SetDefault("job_runner.state_update_timeout_seconds", 10)
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	cfg = &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}
	if err := Validate(cfg); err != nil {
		return err
	}

	return nil
}

func Get() *Config {
	return cfg
}
