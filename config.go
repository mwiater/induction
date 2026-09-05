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

const defaultConfigPath = "induction.yaml"

const defaultSidebarWidth = 32

// Duration is a time.Duration that is represented by strings such as "2s" or
// "20m" in induction.yaml.
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
type LogConfig struct {
	Path         string `yaml:"path"`
	Console      bool   `yaml:"console"`
	Prefix       string `yaml:"prefix"`
	Microseconds bool   `yaml:"microseconds"`
}

// ModelManagerConfig controls model discovery and local model storage.
type ModelManagerConfig struct {
	SearchResults          int      `yaml:"SearchResults" mapstructure:"SearchResults" json:"searchResults"`
	PreferredProviders     []string `yaml:"PreferredProviders" mapstructure:"PreferredProviders" json:"preferredProviders"`
	ModelsPath             string   `yaml:"ModelsPath" mapstructure:"ModelsPath" json:"modelsPath"`
	PreferredQuantizations []string `yaml:"PreferredQuantizations" mapstructure:"PreferredQuantizations" json:"preferredQuantizations,omitempty"`
	IncludePatterns        []string `yaml:"IncludePatterns" mapstructure:"IncludePatterns" json:"includePatterns,omitempty"`
	ExcludePatterns        []string `yaml:"ExcludePatterns" mapstructure:"ExcludePatterns" json:"excludePatterns,omitempty"`
	AvailableRAM           string   `yaml:"AvailableRAM" mapstructure:"AvailableRAM" json:"availableRAM,omitempty"`
	AvailableVRAM          string   `yaml:"AvailableVRAM" mapstructure:"AvailableVRAM" json:"availableVRAM,omitempty"`
}

// MCPServerConfig describes a remote Model Context Protocol server. Allow is
// an explicit server-level allowlist switch; disabled entries are never
// contacted or exposed to a model.
// MCPServerConfig describes one configured Model Context Protocol server.
type MCPServerConfig struct {
	Allow bool   `yaml:"MCPServerAllow"`
	Name  string `yaml:"MCPServerName"`
	URL   string `yaml:"MCPServerURL"`
}

// Config contains runtime settings loaded from induction.yaml.
type Config struct {
	Server                   string             `yaml:"server"`
	Timeout                  Duration           `yaml:"timeout"`
	PollInterval             Duration           `yaml:"poll_interval"`
	LoadWaitInterval         Duration           `yaml:"load_wait_interval"`
	EnableLiveMetricsOverlay bool               `yaml:"enableLiveMetricsOverlay"`
	SidebarWidth             int                `yaml:"sidebarWidth"`
	MCPServers               []MCPServerConfig  `yaml:"MCPServers"`
	Log                      LogConfig          `yaml:"log"`
	ModelManager             ModelManagerConfig `yaml:"ModelManager" mapstructure:"ModelManager"`
}

var (
	configOnce sync.Once
	config     *Config
	configErr  error
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
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	return newClientFromConfig(ctx, cfg, options...), nil
}

func newClientFromConfig(ctx context.Context, cfg *Config, options ...ClientOption) *Client {
	configured := []ClientOption{
		WithPollInterval(time.Duration(cfg.PollInterval)),
		WithLoadWaitInterval(time.Duration(cfg.LoadWaitInterval)),
		WithLiveMetricsOverlay(cfg.EnableLiveMetricsOverlay),
	}
	configured = append(configured, options...)
	return NewClient(ctx, cfg.Server, configured...)
}

func (c *Config) validate() error {
	if c.Server == "" {
		return fmt.Errorf("server is required")
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be greater than zero")
	}
	if c.PollInterval <= 0 {
		return fmt.Errorf("poll_interval must be greater than zero")
	}
	if c.LoadWaitInterval <= 0 {
		return fmt.Errorf("load_wait_interval must be greater than zero")
	}
	if c.Log.Path == "" {
		return fmt.Errorf("log.path is required")
	}
	names := make(map[string]bool, len(c.MCPServers))
	for i, server := range c.MCPServers {
		if strings.TrimSpace(server.Name) == "" {
			return fmt.Errorf("MCPServers[%d].MCPServerName is required", i)
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
			return fmt.Errorf("ModelManager: %w", err)
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
		return fmt.Errorf("SearchResults must be between 1 and 100")
	}
	if strings.TrimSpace(c.ModelsPath) == "" {
		return fmt.Errorf("ModelsPath is required")
	}
	abs, err := filepath.Abs(c.ModelsPath)
	if err != nil {
		return fmt.Errorf("resolve ModelsPath: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return fmt.Errorf("create ModelsPath: %w", err)
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

func parseMemorySize(value string) (int64, error) {
	upper := strings.ToUpper(strings.TrimSpace(value))
	// Match the longest suffix first. Otherwise the shorter B suffix can
	// claim values such as 1KB before KB gets a chance to match.
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
