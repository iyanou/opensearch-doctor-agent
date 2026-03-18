package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type Config struct {
	// OpenSearch cluster connection
	Cluster ClusterConfig `mapstructure:"cluster"`

	// SaaS connection
	SaaS SaaSConfig `mapstructure:"saas"`

	// Agent behavior
	Agent AgentConfig `mapstructure:"agent"`
}

type ClusterConfig struct {
	// Display name shown on the dashboard
	Name string `mapstructure:"name"`
	// e.g. https://localhost:9200
	Endpoint string `mapstructure:"endpoint"`
	// production | staging | development | custom
	Environment string `mapstructure:"environment"`
	// Authentication: use username+password OR api_key (not both)
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	APIKey   string `mapstructure:"api_key"`
	// TLS
	TLSSkipVerify bool   `mapstructure:"tls_skip_verify"`
	CACertPath    string `mapstructure:"ca_cert_path"`
}

type SaaSConfig struct {
	// Base URL of the SaaS API
	APIURL string `mapstructure:"api_url"`
	// Agent API key generated on the dashboard
	APIKey string `mapstructure:"api_key"`
}

type AgentConfig struct {
	// Run interval in minutes (default 360 = 6h)
	IntervalMinutes int `mapstructure:"interval_minutes"`
	// Heartbeat interval in seconds (default 300 = 5m)
	HeartbeatSeconds int `mapstructure:"heartbeat_seconds"`
	// Log file path (default: agent.log in same dir)
	LogFile string `mapstructure:"log_file"`
	// Check categories to enable (empty = all enabled)
	EnabledCategories []string `mapstructure:"enabled_categories"`
}

// Load reads config from the given file path (YAML or JSON).
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)

	// Defaults
	v.SetDefault("agent.interval_minutes", 360)
	v.SetDefault("agent.heartbeat_seconds", 300)
	v.SetDefault("agent.log_file", "agent.log")
	v.SetDefault("cluster.environment", "production")
	v.SetDefault("saas.api_url", "http://localhost:3000")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Cluster.Endpoint == "" {
		return fmt.Errorf("cluster.endpoint is required")
	}
	if c.Cluster.Name == "" {
		return fmt.Errorf("cluster.name is required")
	}
	if c.SaaS.APIKey == "" {
		return fmt.Errorf("saas.api_key is required (generate one on the dashboard)")
	}
	if c.Cluster.Username == "" && c.Cluster.APIKey == "" {
		return fmt.Errorf("cluster auth required: set cluster.username+cluster.password or cluster.api_key")
	}
	return nil
}
