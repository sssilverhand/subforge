package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Xray      BinaryConfig    `yaml:"xray"`
	Hysteria2 BinaryConfig    `yaml:"hysteria2"`
}

type ServerConfig struct {
	Host   string `yaml:"host"`
	Port   int    `yaml:"port"`
	Secret string `yaml:"secret"` // Bearer token for auth
}

type BinaryConfig struct {
	BinaryPath string `yaml:"binary"`  // /usr/local/bin/xray
	ConfigPath string `yaml:"config"`  // /etc/xray/config.json
	Service    string `yaml:"service"` // xray.service
}

func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open agent config: %w", err)
	}
	defer f.Close()

	cfg := &Config{}
	if err := yaml.NewDecoder(f).Decode(cfg); err != nil {
		return nil, fmt.Errorf("parse agent config: %w", err)
	}
	cfg.setDefaults()
	return cfg, cfg.validate()
}

func (c *Config) setDefaults() {
	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 9090
	}
	if c.Xray.BinaryPath == "" {
		c.Xray.BinaryPath = "/usr/local/bin/xray"
	}
	if c.Xray.ConfigPath == "" {
		c.Xray.ConfigPath = "/etc/xray/config.json"
	}
	if c.Xray.Service == "" {
		c.Xray.Service = "xray.service"
	}
	if c.Hysteria2.BinaryPath == "" {
		c.Hysteria2.BinaryPath = "/usr/local/bin/hysteria2"
	}
	if c.Hysteria2.ConfigPath == "" {
		c.Hysteria2.ConfigPath = "/etc/hysteria2/config.yaml"
	}
	if c.Hysteria2.Service == "" {
		c.Hysteria2.Service = "hysteria2.service"
	}
}

func (c *Config) validate() error {
	if c.Server.Secret == "" {
		return fmt.Errorf("server.secret is required")
	}
	return nil
}
