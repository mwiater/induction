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
type ReasoningConfig struct {
	Cutoff *ReasoningCutoffConfig `yaml:"cutoff,omitempty" json:"cutoff,omitempty"`
}

// ReasoningCutoffConfig limits the model's reasoning phase. Numeric pointers
// deliberately distinguish an omitted threshold from a configured zero.
type ReasoningCutoffConfig struct {
	Enabled           bool     `yaml:"enabled" json:"enabled"`
	MaxTokens         *int     `yaml:"maxTokens,omitempty" json:"maxTokens,omitempty"`
	MaxSeconds        *float64 `yaml:"maxSeconds,omitempty" json:"maxSeconds,omitempty"`
	MaxContextPercent *float64 `yaml:"maxContextPercent,omitempty" json:"maxContextPercent,omitempty"`
}

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

func isFinite(v float64) bool {
	return v == v && v < 1.7976931348623157e308 && v > -1.7976931348623157e308
}

type ChatCompletionControlRequest struct {
	ID     string `json:"id"`
	Action string `json:"action"`
	Model  string `json:"model,omitempty"`
}
type ChatCompletionControlResponse struct {
	Success bool   `json:"success"`
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

type ReasoningCutoffReason string

const (
	ReasoningCutoffReasonMaxTokens  ReasoningCutoffReason = "max_tokens"
	ReasoningCutoffReasonMaxSeconds ReasoningCutoffReason = "max_seconds"
	ReasoningCutoffReasonMaxContext ReasoningCutoffReason = "max_context_percent"
)

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

func formatCutoffPercent(value *float64) string {
	if value == nil {
		return "unknown"
	}
	return strconv.FormatFloat(*value, 'f', 2, 64) + "%"
}

// ReasoningCutoffSnapshot is safe to copy and is suitable for persistence.
type ReasoningCutoffSnapshot struct {
	Enabled            bool                  `json:"enabled"`
	Triggered          bool                  `json:"triggered"`
	Reason             ReasoningCutoffReason `json:"trigger_reason,omitempty"`
	CompletionID       string                `json:"completion_id,omitempty"`
	ReasoningStartedAt *time.Time            `json:"reasoning_started_at,omitempty"`
	TriggeredAt        *time.Time            `json:"triggered_at,omitempty"`
	ReasoningTokens    int                   `json:"reasoning_tokens,omitempty"`
	ReasoningSeconds   *float64              `json:"reasoning_seconds,omitempty"`
	ContextTokens      *int                  `json:"context_tokens,omitempty"`
	ContextSize        *int                  `json:"context_size,omitempty"`
	ContextPercent     *float64              `json:"context_percent,omitempty"`
	ControlSent        bool                  `json:"control_sent"`
	ControlSuccess     bool                  `json:"control_success"`
	ControlMessage     string                `json:"control_message,omitempty"`
}

// reasoningWatchdog owns all mutable cutoff state. The control callback is
// invoked at most once and runs independently of the SSE reader.
type reasoningWatchdog struct {
	mu             sync.Mutex
	cfg            *ReasoningCutoffConfig
	snapshot       ReasoningCutoffSnapshot
	control        func(string) (*ChatCompletionControlResponse, error)
	timer          *time.Timer
	ended          bool
	stopped        bool
	pending        ReasoningCutoffReason
	metricBaseline *int
}

func newReasoningWatchdog(cfg *ReasoningCutoffConfig, control func(string) (*ChatCompletionControlResponse, error)) *reasoningWatchdog {
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	return &reasoningWatchdog{cfg: cfg, control: control, snapshot: ReasoningCutoffSnapshot{Enabled: true}}
}

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
func (w *reasoningWatchdog) end() {
	w.mu.Lock()
	w.ended = true
	if w.timer != nil {
		w.timer.Stop()
	}
	w.mu.Unlock()
}
func (w *reasoningWatchdog) stop() {
	w.mu.Lock()
	w.stopped = true
	if w.timer != nil {
		w.timer.Stop()
	}
	w.mu.Unlock()
}
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
func (w *reasoningWatchdog) snapshotCopy() ReasoningCutoffSnapshot {
	if w == nil {
		return ReasoningCutoffSnapshot{}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.snapshot
}
