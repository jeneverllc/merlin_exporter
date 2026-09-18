package main

import (
	"fmt"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/pkg/errors"
)

// Merlin represents the portal endpoint and credentials.
type Merlin struct {
	URL string `yaml:"url" env:"MERLIN_URL" env-default:"https://pa.speedtest.rcn.net/merlin"`
}

// Telemetry represents the exporter's listen address and metrics URI path.
type Telemetry struct {
	ListenAddress string `yaml:"listen_address" env:"TELEMETRY_LISTEN_ADDRESS" env-default:"0.0.0.0:9528"`
	MetricsPath   string `yaml:"metrics_path" env:"TELEMETRY_METRICS_PATH" env-default:"/metrics"`
}

// Config represents the YAML config file structure.
type Config struct {
	Merlin    Merlin    `yaml:"merlin"`
	Telemetry Telemetry `yaml:"telemetry"`
}

// NewConfigFromFile reads the configuration file from the given path
// and returns a populated Config struct.
func NewConfigFromFile(path string) (*Config, error) {
	// Setup default config.
	config := Config{
		Telemetry: Telemetry{
			ListenAddress: ":9527",
			MetricsPath:   "/metrics",
		},
	}

	err := cleanenv.ReadConfig(path, &config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read config file")
	}
	if config.Merlin.URL == "" {
		return nil, fmt.Errorf("Merlin URL isn't set in config")
	}

	return &config, nil
}
