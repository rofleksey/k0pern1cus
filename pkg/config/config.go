package config

import (
	"fmt"
	"os"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Log struct {
		Telegram struct {
			Token  string `yaml:"token"`
			ChatID string `yaml:"chat_id"`
		} `yaml:"telegram"`
	} `yaml:"log"`

	Sentry struct {
		DSN              string  `yaml:"dsn"`
		Environment      string  `yaml:"environment"`
		TracesSampleRate float64 `yaml:"traces_sample_rate"`
	} `yaml:"sentry"`

	Twitch struct {
		BroadcasterIDs []string `yaml:"broadcaster_ids" validate:"required"`
		GameID         string   `yaml:"game_id" validate:"required"`
		MinDate        string   `yaml:"min_date" validate:"required"`
		ClientID       string   `yaml:"client_id" validate:"required"`
		ClientSecret   string   `yaml:"client_secret" validate:"required"`
		RTMPUrl        string   `yaml:"rtmp_url"`
	} `yaml:"twitch"`
}

func Load() (*Config, error) {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var result Config
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	if result.Sentry.TracesSampleRate == 0 {
		result.Sentry.TracesSampleRate = 1.0
	}
	if result.Sentry.Environment == "" {
		result.Sentry.Environment = "production"
	}

	validate := validator.New(validator.WithRequiredStructEnabled())
	if err := validate.Struct(result); err != nil {
		return nil, fmt.Errorf("failed to validate config: %w", err)
	}

	return &result, nil
}
