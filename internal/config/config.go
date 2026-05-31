package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
	Bot      BotConfig      `yaml:"bot"`
	Scheduler SchedulerConfig `yaml:"scheduler"`
}

type ServerConfig struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	ExternalURL string `yaml:"external_url"` // https://sub.ihne.online
}

type DatabaseConfig struct {
	DSN string `yaml:"dsn"` // postgres://user:pass@host/db
}

type AuthConfig struct {
	JWTSecret       string        `yaml:"jwt_secret"`
	TokenExpiry     time.Duration `yaml:"token_expiry"`
	SuperAdminSetup bool          `yaml:"super_admin_setup"` // allow first-run setup
}

type BotConfig struct {
	Enabled bool   `yaml:"enabled"`
	Token   string `yaml:"token"`
	Webhook string `yaml:"webhook"` // if empty — long polling
}

type SchedulerConfig struct {
	TrafficPollInterval time.Duration `yaml:"traffic_poll_interval"` // default 60s
	ExpiryCheckInterval time.Duration `yaml:"expiry_check_interval"` // default 5m
}

func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	cfg := &Config{}
	if err := yaml.NewDecoder(f).Decode(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.setDefaults()
	return cfg, cfg.validate()
}

func (c *Config) setDefaults() {
	if c.Server.Host == "" {
		c.Server.Host = "127.0.0.1"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Auth.TokenExpiry == 0 {
		c.Auth.TokenExpiry = 24 * time.Hour
	}
	if c.Scheduler.TrafficPollInterval == 0 {
		c.Scheduler.TrafficPollInterval = 60 * time.Second
	}
	if c.Scheduler.ExpiryCheckInterval == 0 {
		c.Scheduler.ExpiryCheckInterval = 5 * time.Minute
	}
}

func (c *Config) validate() error {
	if c.Database.DSN == "" {
		return fmt.Errorf("database.dsn is required")
	}
	if c.Server.ExternalURL == "" {
		return fmt.Errorf("server.external_url is required")
	}
	if c.Auth.JWTSecret == "" {
		return fmt.Errorf("auth.jwt_secret is required")
	}
	return nil
}
