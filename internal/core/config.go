// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// defaultConfigPath identifies a fixed value used internally by this package.
const defaultConfigPath = "induction.yaml"

// defaultSidebarWidth identifies a fixed value used internally by this package.
const defaultSidebarWidth = 32

// Duration is a time.Duration that is represented by strings such as "2s" or
// "20m" in induction.yaml.
// Duration is an exported type in this package's public data model.
type Duration time.Duration

// UnmarshalYAML parses a Go duration string from YAML.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	parsed, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value.Value, err)
	}
	*d = Duration(parsed)
	return nil
}

// LogConfig controls application log destination and formatting.
// LogConfig is an exported type in this package's public data model.
type LogConfig struct {
	// Path stores the value represented by this member.
	Path string `yaml:"path"`
	// Console stores the value represented by this member.
	Console bool `yaml:"console"`
	// Prefix stores the value represented by this member.
	Prefix string `yaml:"prefix"`
	// Microseconds stores the value represented by this member.
	Microseconds bool `yaml:"microseconds"`
	// TruncateOnRun stores the value represented by this member.
	TruncateOnRun bool `yaml:"truncateOnRun"`
}

// ModelManagerConfig controls model discovery and local model storage.
// ModelManagerConfig is an exported type in this package's public data model.
type ModelManagerConfig struct {
	// SearchResults stores the value represented by this member.
	SearchResults int `yaml:"searchResults" mapstructure:"searchResults" json:"searchResults"`
	// PreferredProviders stores the value represented by this member.
	PreferredProviders []string `yaml:"preferredProviders" mapstructure:"preferredProviders" json:"preferredProviders"`
	// ModelsPath stores the value represented by this member.
	ModelsPath string `yaml:"modelsPath" mapstructure:"modelsPath" json:"modelsPath"`
	// PreferredQuantizations stores the value represented by this member.
	PreferredQuantizations []string `yaml:"preferredQuantizations" mapstructure:"preferredQuantizations" json:"preferredQuantizations,omitempty"`
	// IncludePatterns stores the value represented by this member.
	IncludePatterns []string `yaml:"includePatterns" mapstructure:"includePatterns" json:"includePatterns,omitempty"`
	// ExcludePatterns stores the value represented by this member.
	ExcludePatterns []string `yaml:"excludePatterns" mapstructure:"excludePatterns" json:"excludePatterns,omitempty"`
	// AvailableRAM stores the value represented by this member.
	AvailableRAM string `yaml:"availableRAM" mapstructure:"availableRAM" json:"availableRAM,omitempty"`
	// AvailableVRAM stores the value represented by this member.
	AvailableVRAM string `yaml:"availableVRAM" mapstructure:"availableVRAM" json:"availableVRAM,omitempty"`
	// HuggingFaceToken stores the value represented by this member.
	HuggingFaceToken string `yaml:"huggingFaceToken" mapstructure:"huggingFaceToken" json:"-"`
}

// MCPServerConfig describes a remote Model Context Protocol server. Allow is
// an explicit server-level allowlist switch; disabled entries are never
// contacted or exposed to a model.
// MCPServerConfig describes one configured Model Context Protocol server.
// MCPServerConfig is an exported type in this package's public data model.
type MCPServerConfig struct {
	// Allow stores the value represented by this member.
	Allow bool `yaml:"mcpServerAllow"`
	// Name stores the value represented by this member.
	Name string `yaml:"mcpServerName"`
	// URL stores the value represented by this member.
	URL string `yaml:"mcpServerURL"`
}

// ResourceBudgetConfig contains global inference resource policies.
// ResourceBudgetConfig is an exported type in this package's public data model.
type ResourceBudgetConfig struct {
	// Reasoning stores the value represented by this member.
	Reasoning *ReasoningConfig `yaml:"reasoning,omitempty" json:"reasoning,omitempty"`
}

// Config contains runtime settings loaded from induction.yaml.
// Config is an exported type in this package's public data model.
type Config struct {
	// Server stores the value represented by this member.
	Server string `yaml:"server"`
	// Timeout stores the value represented by this member.
	Timeout Duration `yaml:"timeout"`
	// PollInterval stores the value represented by this member.
	PollInterval Duration `yaml:"pollInterval"`
	// LoadWaitInterval stores the value represented by this member.
	LoadWaitInterval Duration `yaml:"loadWaitInterval"`
	// SidebarWidth stores the value represented by this member.
	SidebarWidth int `yaml:"sidebarWidth"`
	// MCPServers stores the value represented by this member.
	MCPServers []MCPServerConfig `yaml:"mcpServers"`
	// Log stores the value represented by this member.
	Log LogConfig `yaml:"log"`
	// ModelManager stores the value represented by this member.
	ModelManager ModelManagerConfig `yaml:"modelManager" mapstructure:"modelManager"`
	// ResourceBudget stores the value represented by this member.
	ResourceBudget *ResourceBudgetConfig `yaml:"resourceBudget,omitempty" json:"resourceBudget,omitempty"`
}

var (
	// configOnce holds package-level state used internally by this package.
	configOnce sync.Once
	// config holds package-level state used internally by this package.
	config *Config
	// configErr holds package-level state used internally by this package.
	configErr error
)

// LoadConfig loads induction.yaml once and returns the process-wide config.
// Without an explicit path, induction.yaml must exist in the current working
// directory (normally the project root). An optional path may be supplied; the
// path from the first call is the one used for the lifetime of the process.
func LoadConfig(path ...string) (*Config, error) {
	configPath := defaultConfigPath
	if len(path) > 0 && path[0] != "" {
		configPath = path[0]
	}

	configOnce.Do(func() {
		contents, err := os.ReadFile(configPath)
		if err != nil {
			configErr = fmt.Errorf("load induction config %q: %w", configPath, err)
			return
		}

		loaded := new(Config)
		decoder := yaml.NewDecoder(strings.NewReader(string(contents)))
		decoder.KnownFields(true)
		if err := decoder.Decode(loaded); err != nil {
			configErr = fmt.Errorf("parse induction config %q: %w", configPath, err)
			return
		}
		if loaded.SidebarWidth <= 0 {
			loaded.SidebarWidth = defaultSidebarWidth
		}
		if err := loaded.validate(); err != nil {
			configErr = fmt.Errorf("validate induction config %q: %w", configPath, err)
			return
		}
		config = loaded
	})

	return config, configErr
}

// NewClientFromConfig loads induction.yaml and constructs a client using its
// server and interval settings. Additional options override YAML settings.
func NewClientFromConfig(ctx context.Context, options ...ClientOption) (*Client, error) {
	configPath := configPathFromOptions(options)
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	return newClientFromConfig(ctx, cfg, options...), nil
}

// configPathFromOptions performs the internal operation described by its signature.
func configPathFromOptions(options []ClientOption) string {
	for _, option := range options {
		probe := &ClientOptions{}
		option(probe)
		if probe.configPath != "" {
			return probe.configPath
		}
	}
	return ""
}

// loadConfigForOptions performs the internal operation described by its signature.
func loadConfigForOptions(options []ClientOption) (*Config, error) {
	return LoadConfig(configPathFromOptions(options))
}

// newClientFromConfig performs the internal operation described by its signature.
func newClientFromConfig(ctx context.Context, cfg *Config, options ...ClientOption) *Client {
	configured := []ClientOption{
		WithPollInterval(time.Duration(cfg.PollInterval)),
		WithLoadWaitInterval(time.Duration(cfg.LoadWaitInterval)),
		WithLogger(configuredLogger(cfg.Log)),
		withResourceBudget(cfg.ResourceBudget),
	}
	configured = append(configured, options...)
	return NewClient(ctx, cfg.Server, configured...)
}

// validate performs the internal operation described by its signature.
func (c *Config) validate() error {
	if c.Server == "" {
		return fmt.Errorf("server is required")
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be greater than zero")
	}
	if c.PollInterval <= 0 {
		return fmt.Errorf("pollInterval must be greater than zero")
	}
	if c.LoadWaitInterval <= 0 {
		return fmt.Errorf("loadWaitInterval must be greater than zero")
	}
	if c.ResourceBudget != nil && c.ResourceBudget.Reasoning != nil && c.ResourceBudget.Reasoning.Cutoff != nil {
		if err := c.ResourceBudget.Reasoning.Cutoff.validate("resourceBudget.reasoning.cutoff"); err != nil {
			return err
		}
	}
	names := make(map[string]bool, len(c.MCPServers))
	for i, server := range c.MCPServers {
		if strings.TrimSpace(server.Name) == "" {
			return fmt.Errorf("mcpServers[%d].mcpServerName is required", i)
		}
		if names[server.Name] {
			return fmt.Errorf("MCP server name %q is duplicated", server.Name)
		}
		names[server.Name] = true
		parsed, err := url.Parse(server.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("MCP server %q has an invalid HTTP(S) URL", server.Name)
		}
	}
	if c.ModelManager.SearchResults != 0 || c.ModelManager.ModelsPath != "" || len(c.ModelManager.PreferredProviders) > 0 || len(c.ModelManager.PreferredQuantizations) > 0 || len(c.ModelManager.IncludePatterns) > 0 || len(c.ModelManager.ExcludePatterns) > 0 || c.ModelManager.AvailableRAM != "" || c.ModelManager.AvailableVRAM != "" {
		if err := c.ModelManager.NormalizeAndValidate(); err != nil {
			return fmt.Errorf("modelManager: %w", err)
		}
	}
	return nil
}

// NormalizeAndValidate applies defaults and validates model-manager settings.
func (c *ModelManagerConfig) NormalizeAndValidate() error {
	if c.SearchResults == 0 {
		c.SearchResults = 10
	}
	if c.SearchResults < 1 || c.SearchResults > 100 {
		return fmt.Errorf("searchResults must be between 1 and 100")
	}
	if strings.TrimSpace(c.ModelsPath) == "" {
		return fmt.Errorf("modelsPath is required")
	}
	abs, err := filepath.Abs(c.ModelsPath)
	if err != nil {
		return fmt.Errorf("resolve modelsPath: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return fmt.Errorf("create modelsPath: %w", err)
	}
	c.ModelsPath = abs
	seen := make(map[string]struct{}, len(c.PreferredProviders))
	providers := c.PreferredProviders[:0]
	for _, provider := range c.PreferredProviders {
		provider = strings.TrimSpace(provider)
		if provider == "" {
			continue
		}
		key := strings.ToLower(provider)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		providers = append(providers, provider)
	}
	c.PreferredProviders = providers
	for _, group := range [][]string{c.IncludePatterns, c.ExcludePatterns} {
		for _, pattern := range group {
			if _, err := filepath.Match(pattern, ""); err != nil {
				return fmt.Errorf("invalid file pattern %q: %w", pattern, err)
			}
		}
	}
	for label, value := range map[string]string{"AvailableRAM": c.AvailableRAM, "AvailableVRAM": c.AvailableVRAM} {
		if value != "" {
			if _, err := parseMemorySize(value); err != nil {
				return fmt.Errorf("%s: %w", label, err)
			}
		}
	}
	return nil
}

// parseMemorySize performs the internal operation described by its signature.
// Additional context: Match the longest suffix first. Otherwise the shorter B suffix can claim values such as 1KB before KB gets a chance to match.
func parseMemorySize(value string) (int64, error) {
	upper := strings.ToUpper(strings.TrimSpace(value))

	units := []struct {
		suffix     string
		multiplier float64
	}{
		{"KIB", 1 << 10}, {"MIB", 1 << 20}, {"GIB", 1 << 30}, {"TIB", 1 << 40},
		{"KB", 1e3}, {"MB", 1e6}, {"GB", 1e9}, {"TB", 1e12}, {"B", 1},
	}
	for _, unit := range units {
		if strings.HasSuffix(upper, unit.suffix) {
			number := strings.TrimSpace(upper[:len(upper)-len(unit.suffix)])
			parsed, err := strconv.ParseFloat(number, 64)
			if err != nil || parsed < 0 {
				return 0, fmt.Errorf("invalid memory size %q", value)
			}
			return int64(parsed * unit.multiplier), nil
		}
	}
	return 0, fmt.Errorf("memory size %q requires a unit", value)
}

// Validate normalizes and validates all configuration fields.
func (c *Config) Validate() error { return c.validate() }
