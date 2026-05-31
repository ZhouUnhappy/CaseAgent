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
	Provider              string `mapstructure:"provider"`
	Model                 string `mapstructure:"model"`
	APIKey                string `mapstructure:"api_key"`
	AccessKey             string `mapstructure:"access_key"`
	SecretKey             string `mapstructure:"secret_key"`
	BaseURL               string `mapstructure:"base_url"`
	Region                string `mapstructure:"region"`
	RequestTimeoutSeconds int    `mapstructure:"request_timeout_seconds"`
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

var cfg *Config

func Load(configPath string) error {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.SetDefault("model.chat.request_timeout_seconds", 60)
	viper.SetDefault("suggestion.auto_dismiss_pending_days", 30)
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	cfg = &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return nil
}

func Get() *Config {
	return cfg
}
