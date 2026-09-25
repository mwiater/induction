package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Version     int              `yaml:"version" json:"version"`
	Name        string           `yaml:"name" json:"name"`
	Description string           `yaml:"description" json:"description,omitempty"`
	Provider    ProviderConfig   `yaml:"provider" json:"provider"`
	Generation  GenerationConfig `yaml:"generation" json:"generation"`
	Execution   ExecutionConfig  `yaml:"execution" json:"execution"`
	Evals       []Definition     `yaml:"evals" json:"evals"`
}

type ProviderConfig struct {
	Name string `yaml:"name" json:"name"`
}
type GenerationConfig struct {
	Temperature float64 `yaml:"temperature" json:"temperature"`
	MaxTokens   int     `yaml:"max_tokens" json:"max_tokens"`
}
type ExecutionConfig struct {
	FailFast bool `yaml:"fail_fast" json:"fail_fast"`
}
type Definition struct {
	Task               string            `yaml:"task" json:"task"`
	Name               string            `yaml:"name" json:"name"`
	Tag                string            `yaml:"tag" json:"tag"`
	RequiresImageInput bool              `yaml:"requires_image_input,omitempty" json:"requires_image_input,omitempty"`
	Limit              int               `yaml:"limit" json:"limit,omitempty"`
	MaxTokens          int               `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
	TaskArgs           map[string]string `yaml:"task_args,omitempty" json:"task_args,omitempty"`
}

func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load eval config %q: %w", path, err)
	}
	var raw yaml.Node
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse eval config %q: %w", path, err)
	}
	c := new(Config)
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(c); err != nil {
		return nil, fmt.Errorf("parse eval config %q: %w", path, err)
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("validate eval config %q: %w", path, err)
	}
	return c, nil
}

func (c *Config) Validate() error {
	if c.Version <= 0 {
		return fmt.Errorf("version must be greater than zero")
	}
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if len(c.Evals) == 0 {
		return fmt.Errorf("evals must not be empty")
	}
	seen := map[string]bool{}
	seenTags := map[string]bool{}
	for _, e := range c.Evals {
		if strings.TrimSpace(e.Task) == "" {
			return fmt.Errorf("eval task is required")
		}
		if strings.TrimSpace(e.Name) == "" {
			return fmt.Errorf("eval name is required")
		}
		if strings.TrimSpace(e.Tag) == "" {
			return fmt.Errorf("eval %q tag is required", e.Name)
		}
		if strings.ContainsAny(e.Tag, "\\\"';&|$`()< >\n\r\t") {
			return fmt.Errorf("invalid eval tag %q for %q", e.Tag, e.Name)
		}
		if seenTags[e.Tag] {
			return fmt.Errorf("duplicate eval tag: %s", e.Tag)
		}
		seenTags[e.Tag] = true
		if e.Limit < 0 {
			return fmt.Errorf("eval %q limit must not be negative", e.Name)
		}
		if e.MaxTokens < 0 {
			return fmt.Errorf("eval %q max_tokens must not be negative", e.Name)
		}
		if seen[e.Name] {
			return fmt.Errorf("duplicate eval name: %s", e.Name)
		}
		seen[e.Name] = true
		for key, value := range e.TaskArgs {
			if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "=\\\"';&|$`()< >\n\r\t") {
				return fmt.Errorf("invalid task argument name %q for eval %q", key, e.Name)
			}
			if strings.ContainsAny(value, "\x00\n\r") {
				return fmt.Errorf("invalid task argument value for %q", key)
			}
		}
		if !strings.HasPrefix(e.Task, "inspect_evals/") || strings.ContainsAny(e.Task, "\\\"';&|$`()<>\n\r\t ") {
			return fmt.Errorf("invalid inspect task %q", e.Task)
		}
	}
	return nil
}

func (c *Config) Hash() (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:]), nil
}
