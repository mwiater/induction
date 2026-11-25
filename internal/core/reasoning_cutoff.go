// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ReasoningConfig contains per-request reasoning policy.
// ReasoningConfig is an exported type in this package's public data model.
type ReasoningConfig struct {
	// Cutoff stores the value represented by this member.
	Cutoff *ReasoningCutoffConfig `yaml:"cutoff,omitempty" json:"cutoff,omitempty"`
}

// ReasoningCutoffConfig limits the model's reasoning phase. Numeric pointers
// deliberately distinguish an omitted threshold from a configured zero.
// ReasoningCutoffConfig is an exported type in this package's public data model.
type ReasoningCutoffConfig struct {
	// Enabled stores the value represented by this member.
	Enabled bool `yaml:"enabled" json:"enabled"`
	// MaxTokens stores the value represented by this member.
	MaxTokens *int `yaml:"maxTokens,omitempty" json:"maxTokens,omitempty"`
	// MaxSeconds stores the value represented by this member.
	MaxSeconds *float64 `yaml:"maxSeconds,omitempty" json:"maxSeconds,omitempty"`
	// MaxContextPercent stores the value represented by this member.
	MaxContextPercent *float64 `yaml:"maxContextPercent,omitempty" json:"maxContextPercent,omitempty"`
}

// validate performs the internal operation described by its signature.
func (c *ReasoningCutoffConfig) validate(prefix string) error {
	if c == nil || !c.Enabled {
		return nil
	}
	if c.MaxTokens == nil && c.MaxSeconds == nil && c.MaxContextPercent == nil {
		return fmt.Errorf("%s is enabled but no cutoff threshold is configured", prefix)
	}
	if c.MaxTokens != nil && *c.MaxTokens < 1 {
		return fmt.Errorf("%s.maxTokens must be at least 1", prefix)
	}
	if c.MaxSeconds != nil && (*c.MaxSeconds <= 0 || !isFinite(*c.MaxSeconds)) {
		return fmt.Errorf("%s.maxSeconds must be greater than 0", prefix)
	}
	if c.MaxContextPercent != nil && (*c.MaxContextPercent <= 0 || *c.MaxContextPercent > 100 || !isFinite(*c.MaxContextPercent)) {
		return fmt.Errorf("%s.maxContextPercent must be in (0,100]", prefix)
	}
	return nil
}

// isFinite performs the internal operation described by its signature.
func isFinite(v float64) bool {
	return v == v && v < 1.7976931348623157e308 && v > -1.7976931348623157e308
}

// ChatCompletionControlRequest is an exported type in this package's public data model.
type ChatCompletionControlRequest struct {
	// ID stores the value represented by this member.
	ID string `json:"id"`
	// Action stores the value represented by this member.
	Action string `json:"action"`
	// Model stores the value represented by this member.
	Model string `json:"model,omitempty"`
}

// ChatCompletionControlResponse is an exported type in this package's public data model.
type ChatCompletionControlResponse struct {
	// Success stores the value represented by this member.
	Success bool `json:"success"`
	// Message stores the value represented by this member.
	Message string `json:"message,omitempty"`
}

// EndReasoning asks llama.cpp to close reasoning on the active completion.
func (c *Client) EndReasoning(ctx context.Context, model, completionID string) (*ChatCompletionControlResponse, error) {
	if strings.TrimSpace(completionID) == "" {
		return nil, fmt.Errorf("completion ID is required")
	}
	payload, err := json.Marshal(ChatCompletionControlRequest{ID: completionID, Action: "reasoning_end", Model: model})
	if err != nil {
		return nil, fmt.Errorf("marshal reasoning control request: %w", err)
	}
	controlCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(controlCtx, http.MethodPost, c.endpoint+"/v1/chat/completions/control", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.clientHTTP().Do(req)
	if err != nil {
		return nil, fmt.Errorf("reasoning control request failed: %w", err)
	}
	defer resp.Body.Close()
	var result ChatCompletionControlResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode reasoning control response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("reasoning control returned %d: %s", resp.StatusCode, result.Message)
	}
	if !result.Success {
		return &result, fmt.Errorf("reasoning control rejected: %s", result.Message)
	}
	return &result, nil
}

// ReasoningCutoffReason is an exported type in this package's public data model.
type ReasoningCutoffReason string

const (
	// ReasoningCutoffReasonMaxTokens identifies a fixed value used internally by this package.
	ReasoningCutoffReasonMaxTokens ReasoningCutoffReason = "max_tokens"
	// ReasoningCutoffReasonMaxSeconds identifies a fixed value used internally by this package.
	ReasoningCutoffReasonMaxSeconds ReasoningCutoffReason = "max_seconds"
	// ReasoningCutoffReasonMaxContext identifies a fixed value used internally by this package.
	ReasoningCutoffReasonMaxContext ReasoningCutoffReason = "max_context_percent"
)

// reasoningCutoffConfigKey performs the internal operation described by its signature.
func reasoningCutoffConfigKey(reason ReasoningCutoffReason) string {
	switch reason {
	case ReasoningCutoffReasonMaxTokens:
		return "maxTokens"
	case ReasoningCutoffReasonMaxSeconds:
		return "maxSeconds"
	case ReasoningCutoffReasonMaxContext:
		return "maxContextPercent"
	default:
		return string(reason)
	}
}

// formatCutoffPercent performs the internal operation described by its signature.
func formatCutoffPercent(value *float64) string {
	if value == nil {
		return "unknown"
	}
	return strconv.FormatFloat(*value, 'f', 2, 64) + "%"
}

// ReasoningCutoffSnapshot is safe to copy and is suitable for persistence.
// ReasoningCutoffSnapshot is an exported type in this package's public data model.
type ReasoningCutoffSnapshot struct {
	// Enabled stores the value represented by this member.
	Enabled bool `json:"enabled"`
	// Triggered stores the value represented by this member.
	Triggered bool `json:"triggered"`
	// Reason stores the value represented by this member.
	Reason ReasoningCutoffReason `json:"trigger_reason,omitempty"`
	// CompletionID stores the value represented by this member.
	CompletionID string `json:"completion_id,omitempty"`
	// ReasoningStartedAt stores the value represented by this member.
	ReasoningStartedAt *time.Time `json:"reasoning_started_at,omitempty"`
	// TriggeredAt stores the value represented by this member.
	TriggeredAt *time.Time `json:"triggered_at,omitempty"`
	// ReasoningTokens stores the value represented by this member.
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
	// ReasoningSeconds stores the value represented by this member.
	ReasoningSeconds *float64 `json:"reasoning_seconds,omitempty"`
	// ContextTokens stores the value represented by this member.
	ContextTokens *int `json:"context_tokens,omitempty"`
	// ContextSize stores the value represented by this member.
	ContextSize *int `json:"context_size,omitempty"`
	// ContextPercent stores the value represented by this member.
	ContextPercent *float64 `json:"context_percent,omitempty"`
	// ControlSent stores the value represented by this member.
	ControlSent bool `json:"control_sent"`
	// ControlSuccess stores the value represented by this member.
	ControlSuccess bool `json:"control_success"`
	// ControlMessage stores the value represented by this member.
	ControlMessage string `json:"control_message,omitempty"`
}

// reasoningWatchdog owns all mutable cutoff state. The control callback is
// invoked at most once and runs independently of the SSE reader.
// reasoningWatchdog is an internal data type used by this package.
type reasoningWatchdog struct {
	// mu stores the value represented by this member.
	mu sync.Mutex
	// cfg stores the value represented by this member.
	cfg *ReasoningCutoffConfig
	// snapshot stores the value represented by this member.
	snapshot ReasoningCutoffSnapshot
	// control stores the value represented by this member.
	control func(string) (*ChatCompletionControlResponse, error)
	// timer stores the value represented by this member.
	timer *time.Timer
	// ended stores the value represented by this member.
	ended bool
	// stopped stores the value represented by this member.
	stopped bool
	// pending stores the value represented by this member.
	pending ReasoningCutoffReason
	// metricBaseline stores the value represented by this member.
	metricBaseline *int
}

// newReasoningWatchdog performs the internal operation described by its signature.
func newReasoningWatchdog(cfg *ReasoningCutoffConfig, control func(string) (*ChatCompletionControlResponse, error)) *reasoningWatchdog {
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	return &reasoningWatchdog{cfg: cfg, control: control, snapshot: ReasoningCutoffSnapshot{Enabled: true}}
}

// setID performs the internal operation described by its signature.
func (w *reasoningWatchdog) setID(id string) {
	if id == "" {
		return
	}
	w.mu.Lock()
	w.snapshot.CompletionID = id
	pending := w.pending
	w.mu.Unlock()
	if pending != "" {
		w.fire(pending)
	}
}

// start performs the internal operation described by its signature.
func (w *reasoningWatchdog) start() {
	w.mu.Lock()
	if w.stopped || w.ended || w.snapshot.ReasoningStartedAt != nil {
		w.mu.Unlock()
		return
	}
	now := time.Now()
	w.snapshot.ReasoningStartedAt = &now
	duration := time.Duration(0)
	if w.cfg.MaxSeconds != nil {
		duration = time.Duration(*w.cfg.MaxSeconds * float64(time.Second))
	}
	if duration > 0 {
		w.timer = time.AfterFunc(duration, func() { w.fire(ReasoningCutoffReasonMaxSeconds) })
	}
	w.mu.Unlock()
}

// end performs the internal operation described by its signature.
func (w *reasoningWatchdog) end() {
	w.mu.Lock()
	w.ended = true
	if w.timer != nil {
		w.timer.Stop()
	}
	w.mu.Unlock()
}

// stop performs the internal operation described by its signature.
func (w *reasoningWatchdog) stop() {
	w.mu.Lock()
	w.stopped = true
	if w.timer != nil {
		w.timer.Stop()
	}
	w.mu.Unlock()
}

// update performs the internal operation described by its signature.
func (w *reasoningWatchdog) update(tokens int, contextTokens, contextSize *int) {
	w.mu.Lock()
	if tokens > w.snapshot.ReasoningTokens {
		w.snapshot.ReasoningTokens = tokens
	}
	w.snapshot.ContextTokens, w.snapshot.ContextSize = contextTokens, contextSize
	if contextTokens != nil && contextSize != nil && *contextSize > 0 {
		p := float64(*contextTokens) * 100 / float64(*contextSize)
		w.snapshot.ContextPercent = &p
	}
	var reason ReasoningCutoffReason
	if w.cfg.MaxTokens != nil && tokens >= *w.cfg.MaxTokens {
		reason = ReasoningCutoffReasonMaxTokens
	} else if w.cfg.MaxContextPercent != nil && w.snapshot.ContextPercent != nil && *w.snapshot.ContextPercent >= *w.cfg.MaxContextPercent {
		reason = ReasoningCutoffReasonMaxContext
	}
	w.mu.Unlock()
	if reason != "" {
		w.fire(reason)
	}
}

// updateMetrics converts the live slot decoder counter into reasoning tokens.
// The baseline is captured on the first sample after reasoning starts; while
// the stream is in a reasoning block, decoded tokens are reasoning tokens.
func (w *reasoningWatchdog) updateMetrics(generated, contextTokens, contextSize int) {
	w.mu.Lock()
	if w.metricBaseline == nil {
		baseline := generated
		w.metricBaseline = &baseline
	}
	tokens := generated - *w.metricBaseline
	w.mu.Unlock()
	if tokens < 0 {
		tokens = 0
	}
	var contextTokenPtr, contextSizePtr *int
	if contextTokens > 0 {
		contextTokenPtr = &contextTokens
	}
	if contextSize > 0 {
		contextSizePtr = &contextSize
	}
	w.update(tokens, contextTokenPtr, contextSizePtr)
}

// fire performs the internal operation described by its signature.
func (w *reasoningWatchdog) fire(reason ReasoningCutoffReason) {
	w.mu.Lock()
	if w.stopped || w.ended || w.snapshot.Triggered {
		w.mu.Unlock()
		return
	}
	if w.snapshot.CompletionID == "" {
		w.pending = reason
		w.mu.Unlock()
		return
	}
	now := time.Now()
	w.snapshot.Triggered, w.snapshot.Reason, w.snapshot.TriggeredAt = true, reason, &now
	if w.snapshot.ReasoningStartedAt != nil {
		elapsed := now.Sub(*w.snapshot.ReasoningStartedAt).Seconds()
		w.snapshot.ReasoningSeconds = &elapsed
	}
	id := w.snapshot.CompletionID
	w.snapshot.ControlSent = true
	w.mu.Unlock()
	result, err := w.control(id)
	w.mu.Lock()
	defer w.mu.Unlock()
	if err != nil {
		w.snapshot.ControlMessage = err.Error()
		return
	}
	w.snapshot.ControlSuccess = result != nil && result.Success
	if result != nil {
		w.snapshot.ControlMessage = result.Message
	}
}

// snapshotCopy performs the internal operation described by its signature.
func (w *reasoningWatchdog) snapshotCopy() ReasoningCutoffSnapshot {
	if w == nil {
		return ReasoningCutoffSnapshot{}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.snapshot
}
