package induction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

// ClientOptions stores runtime configuration for a Client.
type ClientOptions struct {
	configPath string
	// httpClient performs all HTTP requests.
	httpClient *http.Client
	// pollInterval controls background polling cadence.
	pollInterval time.Duration
	// loadWaitInterval controls retry timing when waiting for a model.
	loadWaitInterval time.Duration
	// logger receives package diagnostic and informational log messages.
	logger Logger
	// liveMetricsOverlayEnabled displays slot-derived inference metrics.
	liveMetricsOverlayEnabled bool
	// liveMetricsOverlay shares a caller-owned overlay across a compound request.
	liveMetricsOverlay *liveMetricsOverlay
	// initialChatPrompt is inserted into the console input when the initial
	// model load completes.
	initialChatPrompt string
	// initialChatPromptAutoSubmit submits initialChatPrompt after it is inserted.
	initialChatPromptAutoSubmit bool
	// initialModelSelection opens the normal model picker before loading a
	// model. It is used by the root positional-chat shortcut.
	initialModelSelection bool
	// autoExitAfterInitialChat exits after the automated turn's session save.
	autoExitAfterInitialChat bool
	// sessionSaved is called after a session has been written successfully.
	sessionSaved func(string)
	// inferenceCompleted receives the completed UI turn and visible content.
	inferenceCompleted func(*ModelSnapshot, string)
	// pipeline drives sequential turns through the console UI.
	pipeline            *Pipeline
	artifactPathContext *ArtifactPathContext
	resourceBudget      *ResourceBudgetConfig
	progressOverlay     *liveMetricsOverlay
	// mcpTools marks snapshots produced by the configured MCP tool loop.
	mcpTools               bool
	mcpToolNames           []string
	applicationToolHandler ApplicationToolHandler
	applicationToolChain   ApplicationToolChain
}

// Logger is the logging contract used by Induction. The standard library's
// log.Logger satisfies this interface, as do many application log adapters.
type Logger interface {
	Printf(format string, args ...interface{})
}

// ClientOption mutates a ClientOptions value during client construction.
type ClientOption func(*ClientOptions)

// Client orchestrates interactions with a local llama.cpp-compatible server.
type Client struct {
	// endpoint is the base URL for the server.
	endpoint string
	// opts stores runtime configuration.
	opts *ClientOptions
	// loadedModel caches the last observed loaded model name.
	loadedModel atomic.Pointer[string]
	// pendingModelLoadDurations carries explicit load operations to the next
	// snapshot client, including clients created by withoutLiveMetricsOverlay.
	pendingModelLoadDurations *sync.Map
	classificationTokens      *sync.Map
	// runtimeMu serializes lifecycle mutations issued by this client.
	runtimeMu sync.Mutex
}

// NewClient initializes and returns a configured Induction client.
func NewClient(ctx context.Context, endpoint string, options ...ClientOption) *Client {
	_ = ctx

	opts := &ClientOptions{
		httpClient:                &http.Client{Timeout: 10 * time.Minute},
		pollInterval:              2 * time.Second,
		loadWaitInterval:          1 * time.Second,
		logger:                    log.New(io.Discard, "", 0),
		liveMetricsOverlayEnabled: true,
	}

	for _, opt := range options {
		opt(opts)
	}

	c := &Client{
		endpoint:                  endpoint,
		opts:                      opts,
		pendingModelLoadDurations: &sync.Map{},
		classificationTokens:      &sync.Map{},
	}

	empty := ""
	c.loadedModel.Store(&empty)

	return c
}

// GenerateSnapshot executes an inference request and collects related telemetry.
func (c *Client) GenerateSnapshot(ctx context.Context, req *ChatRequest) (*ModelSnapshot, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	loadTime, err := c.ensureModelLoaded(ctx, req.Model)
	if err != nil {
		return nil, fmt.Errorf("gatekeeper check failed: %w", err)
	}

	snapshot := &ModelSnapshot{
		ModelID:       req.Model,
		ModelLoadTime: loadTime,
		CollectedAt:   time.Now(),
		Messages:      snapshotMessages(req),
	}
	initializeSnapshotMetadataForMCPWithNames(snapshot, req, c.opts.mcpTools, c.opts.mcpToolNames)

	monitor := c.startInferenceMonitor(ctx, req.Model, true)

	var interaction *Interaction
	if req.Decision != nil || req.Classification != nil {
		interaction, err = c.doDecision(ctx, req)
	} else {
		interaction, err = c.doInference(ctx, req)
	}
	if err == nil && monitor.overlay != nil {
		monitor.overlay.Complete()
	}
	if slots := monitor.Stop(); len(slots) > 0 {
		snapshot.Slots = slots
	}
	if err != nil {
		return nil, fmt.Errorf("inference failed: %w", err)
	}
	if interaction != nil {
		snapshot.Interaction = []Interaction{*interaction}
		snapshot.Messages = append(snapshot.Messages, Message{Role: "assistant", Content: interaction.Content})
	}

	// Collect secondary telemetry concurrently.
	eg, egCtx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		props, err := c.fetchPropsReady(egCtx, req.Model)
		if err != nil {
			c.logf("telemetry: fetch props failed: %v", err)
			return nil
		}
		snapshot.Props = props
		return nil
	})

	eg.Go(func() error {
		slots, err := c.fetchSlotsReady(egCtx, req.Model)
		if err != nil {
			c.logf("telemetry: fetch slots failed: %v", err)
			return nil
		}
		snapshot.Slots = slots
		return nil
	})

	eg.Go(func() error {
		metrics, err := c.fetchMetricsReady(egCtx, req.Model)
		if err != nil {
			c.logf("telemetry: fetch metrics failed: %v", err)
			return nil
		}
		snapshot.Metrics = metrics
		return nil
	})

	_ = eg.Wait()
	snapshot.CollectedAt = time.Now()

	return snapshot, nil
}

// GenerateStreamingSnapshot streams an inference response while collecting
// the same telemetry as GenerateSnapshot.
func (c *Client) GenerateStreamingSnapshot(ctx context.Context, req *ChatRequest, yield func(InferenceStreamChunk) error) (*ModelSnapshot, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if req.Decision != nil || req.Classification != nil {
		return nil, fmt.Errorf("decision inference is non-streaming; use GenerateSnapshot")
	}
	if yield == nil {
		return nil, fmt.Errorf("stream callback is nil")
	}
	loadTime, err := c.ensureModelLoaded(ctx, req.Model)
	if err != nil {
		return nil, fmt.Errorf("gatekeeper check failed: %w", err)
	}
	snapshot := &ModelSnapshot{ModelID: req.Model, ModelLoadTime: loadTime, CollectedAt: time.Now(), Messages: snapshotMessages(req)}
	initializeSnapshotMetadataForMCPWithNames(snapshot, req, c.opts.mcpTools, c.opts.mcpToolNames)
	monitor := c.startInferenceMonitor(ctx, req.Model, true)
	var watchdog *reasoningWatchdog
	if c.opts.resourceBudget != nil && c.opts.resourceBudget.Reasoning != nil && c.opts.resourceBudget.Reasoning.Cutoff != nil && c.opts.resourceBudget.Reasoning.Cutoff.Enabled {
		enabled := true
		request := cloneChatRequest(req)
		request.ReasoningControl = &enabled
		req = &request
		watchdog = nil
		watchdog = newReasoningWatchdog(c.opts.resourceBudget.Reasoning.Cutoff, func(id string) (*ChatCompletionControlResponse, error) {
			// The active SSE request may be cancelled as soon as it finishes; the
			// control call has its own short deadline and must not inherit that
			// cancellation.
			snapshot := watchdog.snapshotCopy()
			c.logf("resource budget hit: reasoning cutoff key=%s model=%q completion=%q reasoning_tokens=%d context_percent=%s", reasoningCutoffConfigKey(snapshot.Reason), req.Model, id, snapshot.ReasoningTokens, formatCutoffPercent(snapshot.ContextPercent))
			result, err := c.EndReasoning(context.Background(), req.Model, id)
			if err != nil {
				c.logf("reasoning cutoff control failed model=%q completion=%q error=%v", req.Model, id, err)
			} else {
				c.logf("reasoning cutoff control accepted model=%q completion=%q", req.Model, id)
			}
			return result, err
		})
	}
	var content, reasoning strings.Builder
	metricsWarningLogged := false
	reasoningActive := false
	sawReasoning := false
	reasoningTokens := 0
	chunks := make([]InferenceStreamChunk, 0)
	err = c.inferStreamChunks(ctx, req, func(chunk InferenceStreamChunk) error {
		if watchdog != nil && chunk.ID != "" {
			watchdog.setID(chunk.ID)
		}
		chunks = append(chunks, chunk)
		for _, choice := range chunk.Choices {
			if choice.Delta.ReasoningContent != "" {
				reasoningActive, sawReasoning = true, true
				if overlay := c.progressOverlay(); overlay != nil {
					overlay.SetReasoningPhase(true)
				}
				metricsGenerated, metricsUsed, metricsCapacity, metricsOK := monitor.latestMetrics()
				if metricsOK && metricsGenerated > reasoningTokens {
					reasoningTokens = metricsGenerated
				}
				if watchdog != nil {
					watchdog.start()
					reasoning.WriteString(choice.Delta.ReasoningContent)
					if choice.Delta.ReasoningTokens != nil {
						watchdog.update(*choice.Delta.ReasoningTokens, nil, nil)
						if *choice.Delta.ReasoningTokens > reasoningTokens {
							reasoningTokens = *choice.Delta.ReasoningTokens
						}
					} else if choice.Logprobs != nil {
						// Some OpenAI-compatible servers expose one logprob
						// position per generated token rather than a dedicated
						// reasoning_tokens field.
						tokens := watchdog.snapshotCopy().ReasoningTokens + len(choice.Logprobs.Content)
						watchdog.update(tokens, nil, nil)
						if tokens > reasoningTokens {
							reasoningTokens = tokens
						}
					} else if metricsOK {
						watchdog.updateMetrics(metricsGenerated, metricsUsed, metricsCapacity)
					} else {
						if !metricsWarningLogged {
							c.logf("reasoning cutoff waiting for live slot metrics model=%q", req.Model)
							metricsWarningLogged = true
						}
					}
				}
				if watchdog == nil {
					reasoning.WriteString(choice.Delta.ReasoningContent)
				}
			}
			text := choice.Delta.Content
			if text == "" {
				text = choice.Text
			}
			if text != "" && watchdog != nil {
				watchdog.end()
			}
			if text != "" {
				reasoningActive = false
				if overlay := c.progressOverlay(); overlay != nil {
					overlay.SetReasoningPhase(false)
				}
				if generated, _, _, ok := monitor.latestMetrics(); ok && generated > reasoningTokens {
					reasoningTokens = generated
				}
			}
			content.WriteString(text)
		}
		return yield(chunk)
	})
	if watchdog != nil {
		watchdog.stop()
		cutoff := watchdog.snapshotCopy()
		snapshot.ReasoningCutoff = &cutoff
		// The watchdog may have a more recent metric sample than the monitor
		// callback observed while parsing the stream. Preserve that authoritative
		// reasoning count for the persisted token split.
		if cutoff.ReasoningTokens > reasoningTokens {
			reasoningTokens = cutoff.ReasoningTokens
		}
	}
	if generated, _, _, ok := monitor.latestMetrics(); ok {
		if generated > reasoningTokens && reasoningActive {
			reasoningTokens = generated
		}
		if sawReasoning {
			value := reasoningTokens
			snapshot.ReasoningTokens = &value
		}
		if !reasoningActive && generated >= reasoningTokens {
			value := generated - reasoningTokens
			snapshot.ResponseTokens = &value
		}
	}
	if snapshot.ResponseTokens == nil && !sawReasoning {
		if generated, _, _, ok := monitor.latestMetrics(); ok {
			value := generated
			snapshot.ResponseTokens = &value
		}
	}
	snapshot.Slots = monitor.Stop()
	if err != nil {
		return nil, fmt.Errorf("inference failed: %w", err)
	}
	rawResponse, _ := json.Marshal(chunks)
	interaction := Interaction{Content: content.String(), ReasoningContent: reasoning.String(), Response: string(rawResponse)}
	snapshot.Interaction = []Interaction{interaction}
	snapshot.Messages = append(snapshot.Messages, Message{Role: "assistant", Content: interaction.Content})

	eg, egCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		props, err := c.fetchPropsReady(egCtx, req.Model)
		if err == nil {
			snapshot.Props = props
		}
		return nil
	})
	eg.Go(func() error {
		metrics, err := c.fetchMetricsReady(egCtx, req.Model)
		if err == nil {
			snapshot.Metrics = metrics
		}
		return nil
	})
	eg.Go(func() error {
		slots, err := c.fetchSlotsReady(egCtx, req.Model)
		if err == nil && len(slots) > 0 {
			snapshot.Slots = slots
		}
		return nil
	})
	_ = eg.Wait()
	snapshot.CollectedAt = time.Now()
	return snapshot, nil
}

// pollSlots samples /slots at the configured cadence until inference ends and
// retains only the newest payload.
func (c *Client) pollSlots(ctx context.Context, model string, overlay *liveMetricsOverlay, updates ...func(SlotsData)) SlotsData {
	if c.opts.pollInterval <= 0 {
		return nil
	}

	ticker := time.NewTicker(c.opts.pollInterval)
	defer ticker.Stop()

	var latest SlotsData
	for {
		select {
		case <-ctx.Done():
			return latest
		case <-ticker.C:
			slots, err := c.fetchSlotsReady(ctx, model)
			if err != nil {
				if ctx.Err() == nil {
					c.logf("telemetry: poll slots failed: %v", err)
				}
				continue
			}
			latest = slots
			for _, update := range updates {
				update(slots)
			}
			// Publish the same samples consumed by the live overlay so active
			// watchdogs can use existing metrics without extra requests.
			// The monitor owns the publication lock; pollSlots itself remains
			// reusable by non-snapshot callers.
			if overlay != nil {
				overlay.updateForModel(model, slots)
			}
		}
	}
}

// loadModel asks the server to begin loading model before the first inference.
func (c *Client) loadModel(ctx context.Context, model string) error {
	start := time.Now()
	// Only the client's own cache can prove that this client already loaded the
	// target. A fresh client must issue an explicit load request: some servers
	// report installed/known models as "loaded" even when max_models=1 has
	// evicted the model from memory.
	current := c.loadedModel.Load()
	if current != nil && *current == model {
		c.loadedModel.Store(&model)
		return c.warmupModel(ctx, model)
	}

	payload, err := json.Marshal(map[string]string{"model": model})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/models/load", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.clientHTTP().Do(req)
	if err != nil {
		return fmt.Errorf("load model request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		// Older or proxy-style servers may not implement explicit model loads.
		// Retain compatibility when the target is already resident according to
		// the legacy model listing.
		if loaded, checkErr := c.modelIsLoaded(ctx, model); checkErr == nil && loaded {
			c.loadedModel.Store(&model)
			return c.warmupModel(ctx, model)
		}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		// A few compatible servers reject an explicit load when the target is
		// already resident. The model listing is authoritative for that case,
		// and lets the UI proceed just as an inference request would.
		if loaded, checkErr := c.modelIsLoaded(ctx, model); checkErr == nil && loaded {
			c.loadedModel.Store(&model)
			return c.warmupModel(ctx, model)
		}
		body, _ := io.ReadAll(resp.Body)
		message := strings.TrimSpace(string(body))
		if message != "" {
			return fmt.Errorf("load model request returned %d: %s", resp.StatusCode, message)
		}
		return fmt.Errorf("load model request returned %d", resp.StatusCode)
	}

	interval := c.opts.loadWaitInterval
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		// Keep polling the lifecycle API used by the model picker. Some
		// servers update /models before /v1/models, while others expose the
		// loaded state only in the runtime record.
		if status, statusErr := c.GetRuntimeStatus(ctx); statusErr == nil {
			for _, runtimeModel := range status.Models {
				if runtimeModel.ID == model && runtimeModel.State == ModelRuntimeLoaded {
					c.loadedModel.Store(&model)
					c.recordModelLoadDuration(model, time.Since(start))
					return c.warmupModel(ctx, model)
				}
			}
		}
		loaded, checkErr := c.modelIsLoaded(ctx, model)
		if checkErr == nil && loaded {
			c.loadedModel.Store(&model)
			c.recordModelLoadDuration(model, time.Since(start))
			return c.warmupModel(ctx, model)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for model %q to load: %w", model, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (c *Client) recordModelLoadDuration(model string, duration time.Duration) {
	if c.pendingModelLoadDurations == nil {
		c.pendingModelLoadDurations = &sync.Map{}
	}
	c.pendingModelLoadDurations.Store(strings.TrimSpace(model), duration)
}

// warmupModel performs one deliberately minimal chat completion after a model
// becomes ready. llama.cpp populates /slots only after inference has started,
// so this primes the slot before the UI reads its model information.
func (c *Client) warmupModel(ctx context.Context, model string) error {
	maxTokens := 1
	_, err := c.infer(ctx, &ChatRequest{
		Model:     model,
		Messages:  []Message{{Role: "user", Content: "1"}},
		MaxTokens: &maxTokens,
	})
	if err != nil {
		return fmt.Errorf("warm up model %q: %w", model, err)
	}
	return nil
}

func (c *Client) modelIsLoaded(ctx context.Context, model string) (bool, error) {
	rows, err := c.fetchModelRows(ctx)
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		if row.Name == model {
			return row.Loaded, nil
		}
	}
	return false, fmt.Errorf("model %q was not found", model)
}

// logf writes a message to the configured application logger.
func (c *Client) logf(format string, args ...interface{}) {
	if c != nil && c.opts != nil && c.opts.logger != nil {
		c.opts.logger.Printf(format, args...)
	}
}

// ensureModelLoaded loads the requested model when the server exposes its
// lifecycle API and returns the measured load transition duration.
func (c *Client) ensureModelLoaded(ctx context.Context, targetModel string) (time.Duration, error) {
	targetModel = strings.TrimSpace(targetModel)
	if targetModel == "" {
		return 0, fmt.Errorf("request model is required")
	}
	var pendingLoad time.Duration
	if c.pendingModelLoadDurations != nil {
		if value, ok := c.pendingModelLoadDurations.LoadAndDelete(targetModel); ok {
			pendingLoad = value.(time.Duration)
		}
	}

	// Every inference must establish the server-authoritative model state. The
	// local cache cannot detect models loaded by another client or by the
	// server itself, so it must not bypass this check.
	_, statusErr := c.GetRuntimeStatus(ctx)
	if statusErr != nil && !runtimeLifecycleUnavailable(statusErr) {
		return 0, fmt.Errorf("inspect model state before inference: %w", statusErr)
	}
	if statusErr == nil {
		result, switchErr := c.SwitchModel(ctx, targetModel)
		if switchErr != nil {
			return 0, fmt.Errorf("switch model before inference: %w", switchErr)
		}
		c.loadedModel.Store(&targetModel)
		if pendingLoad > 0 {
			return pendingLoad, nil
		}
		if result.Load != nil && result.Load.Changed {
			return result.Load.Duration, nil
		}
		return 0, nil
	}

	// When the server exposes the runtime lifecycle API, use its measured
	// operation duration. This is the actual load transition, rather than the
	// time spent updating the client's local model cache.
	operation, err := c.changeModelState(ctx, targetModel, ModelRuntimeLoaded)
	if err == nil {
		c.loadedModel.Store(&targetModel)
		if operation.Changed {
			return operation.Duration, nil
		}
		return 0, nil
	}
	if !runtimeLifecycleUnavailable(err) {
		return 0, fmt.Errorf("load model %q: %w", targetModel, err)
	}

	// Older or proxy-style servers select models from the inference request and
	// do not expose a lifecycle API. Preserve that compatibility, but do not
	// claim a model-load duration that was not measured.
	c.loadedModel.Store(&targetModel)
	return pendingLoad, nil
}

func runtimeLifecycleUnavailable(err error) bool {
	return errors.Is(err, ErrRuntimeUnsupported) || strings.Contains(err.Error(), "models request failed:")
}
