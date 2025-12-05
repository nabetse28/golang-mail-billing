// config/config.go
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the top-level configuration.
type Config struct {
	Gmail GmailConfig `yaml:"gmail"`
	Paths PathsConfig `yaml:"paths"`
}

// GmailConfig holds Gmail-related configuration.
type GmailConfig struct {
	User       string `yaml:"user"`        // typically "me"
	Query      string `yaml:"query"`       // e.g. "has:attachment"
	MaxResults int64  `yaml:"-"` // how many messages to fetch per page for tests
	MaxResultsRaw    any    `yaml:"max_results"` // could be int or "all"
	BaseBillingLabel string `yaml:"base_billing_label"` // e.g. "Facturacion"
	ForceReprocess bool `yaml:"force_reprocess"`

	// Optional filters for downloading attachments.
	// If zero, the filter is ignored.
	FilterYear  int `yaml:"filter_year"`  // e.g. 2025
	FilterMonth int `yaml:"filter_month"` // 1-12 (1=January, 11=November, etc.)
	DownloadOnly bool `yaml:"download_only"` // if true, do not modify labels/read status
}

// PathsConfig holds filesystem paths.
type PathsConfig struct {
	BaseInvoicesPath string `yaml:"base_invoices_path"` // e.g. ~/Documents/Personal/Facturas
}

// Load reads the YAML configuration file from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML config: %w", err)
	}

	if err := cfg.Gmail.Resolve(); err != nil {
    	return nil, err
	}

	applyDefaults(&cfg)

	return &cfg, nil
}

// applyDefaults sets some sane defaults if fields are empty.
func applyDefaults(cfg *Config) {
	if cfg.Gmail.User == "" {
		cfg.Gmail.User = "me"
	}
	if cfg.Gmail.Query == "" {
		cfg.Gmail.Query = "has:attachment"
	}
	if cfg.Gmail.MaxResults == 0 {
		cfg.Gmail.MaxResults = 10
	}
	if cfg.Gmail.BaseBillingLabel == "" {
		cfg.Gmail.BaseBillingLabel = "Facturacion"
	}
}

func (c *GmailConfig) Resolve() error {
    switch v := c.MaxResultsRaw.(type) {
    case int:
        c.MaxResults = int64(v)
    case string:
        if strings.ToLower(v) == "all" {
            c.MaxResults = 500 // Gmail max page size
        } else {
            return fmt.Errorf("invalid max_results value: %s", v)
        }
    default:
        return fmt.Errorf("unsupported type for max_results")
    }

    return nil
}
