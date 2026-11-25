// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/pterm/pterm"
)

// liveMetricsOverlay renders rates computed from the /slots samples that are
// already collected by GenerateSnapshot. It performs no polling of its own.
// liveMetricsOverlay is an internal data type used by this package.
type liveMetricsOverlay struct {
	// mu stores the value represented by this member.
	mu sync.Mutex
	// area stores the value represented by this member.
	area *pterm.AreaPrinter
	// footer stores the value represented by this member.
	footer *stickyFooter
	// startedAt stores the value represented by this member.
	startedAt time.Time
	// lastAt stores the value represented by this member.
	lastAt time.Time
	// lastPrompt stores the value represented by this member.
	lastPrompt float64
	// lastGenerated stores the value represented by this member.
	lastGenerated float64
	// reasoningTokens stores the value represented by this member.
	reasoningTokens float64
	// responseTokens stores the value represented by this member.
	responseTokens float64
	// reasoningPhase stores the value represented by this member.
	reasoningPhase bool
	// lastPrefillRate stores the value represented by this member.
	lastPrefillRate float64
	// lastDecodeRate stores the value represented by this member.
	lastDecodeRate float64
	// lastContextPercent stores the value represented by this member.
	lastContextPercent float64
	// hasMeasurement stores the value represented by this member.
	hasMeasurement bool
	// loading stores the value represented by this member.
	loading bool
	// model stores the value represented by this member.
	model string
	// stopped stores the value represented by this member.
	stopped bool
	// mcp stores the value represented by this member.
	mcp string
	// notify mirrors plain, structured terminal updates to a Bubble Tea program.
	// The legacy footer remains available for non-chat inference entry points.
	notify func(overlayUpdate)
}

// startMCPMetricsOverlay performs the internal operation described by its signature.
func startMCPMetricsOverlay(model string) *liveMetricsOverlay {
	overlay := &liveMetricsOverlay{startedAt: time.Now(), model: model, mcp: "  [Induction: MCP] Initializing MCP tools "}
	if footer, ok := newStickyFooterRows(os.Stdout, 2); ok {
		overlay.footer = footer
		overlay.render(formatFooter(formatLiveMetricsPlain(model, "Prefill", 0, nil, 0, 0)))
		registerLiveMetricsOverlay(overlay)
		return overlay
	}
	return nil
}

// overlayUpdate is an internal data type used by this package.
type overlayUpdate struct {
	// model stores the value represented by this member.
	model string
	// footer stores the value represented by this member.
	footer string
	// slots stores the value represented by this member.
	slots SlotsData
	// modelReady stores the value represented by this member.
	modelReady bool
}

// startLiveMetricsOverlay performs the internal operation described by its signature.
func startLiveMetricsOverlay(model string) *liveMetricsOverlay {
	overlay := &liveMetricsOverlay{startedAt: time.Now(), model: model}
	if footer, ok := newStickyFooter(os.Stdout); ok {
		overlay.footer = footer
		footer.Update(formatLiveMetrics(model, "Prefill", 0, nil, 0, 0))
		registerLiveMetricsOverlay(overlay)
		return overlay
	}
	area, _ := pterm.DefaultArea.WithRemoveWhenDone().Start(formatLiveMetrics(model, "Prefill", 0, nil, 0, 0))
	overlay.area = area
	registerLiveMetricsOverlay(overlay)
	return overlay
}

// Update is an exported operation provided by this package.
func (o *liveMetricsOverlay) Update(slots SlotsData) {
	o.updateForModel("", slots)
}

// updateForModel performs the internal operation described by its signature.
// Additional context: The sidebar still needs idle slot details even when no slot can produce an active-inference rate measurement.
func (o *liveMetricsOverlay) updateForModel(model string, slots SlotsData) {
	prompt, generated, used, capacity, ok := activeSlotMetrics(slots)
	if !ok {

		if o.notify != nil {
			o.notify(overlayUpdate{model: o.model, slots: slots})
		}
		return
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	if model != "" && o.model != model || o.stopped {
		return
	}
	if o.loading {
		return
	}

	now := time.Now()
	previousAt := o.startedAt
	previousPrompt := 0.0
	previousGenerated := 0.0
	if o.hasMeasurement {
		previousAt = o.lastAt
		previousPrompt = o.lastPrompt
		previousGenerated = o.lastGenerated
	}
	elapsed := now.Sub(previousAt).Seconds()
	promptRate, generatedRate := 0.0, 0.0
	if elapsed > 0 {
		promptRate = nonNegative(prompt-previousPrompt) / elapsed
		generatedRate = nonNegative(generated-previousGenerated) / elapsed
	}
	stage := "Prefill"
	var decodeRate *float64
	if generated > 0 {
		stage = "Decode"
		if generatedRate > 0 {
			o.lastDecodeRate = generatedRate
		}
		decodeRate = &o.lastDecodeRate
	} else if promptRate > 0 {
		o.lastPrefillRate = promptRate
	}
	contextPercent := 0.0
	if capacity > 0 {
		contextPercent = used / capacity * 100
	}

	o.lastContextPercent = contextPercent
	prefillRate := o.lastPrefillRate
	if o.reasoningPhase {
		o.reasoningTokens = generated
	} else if generated >= o.reasoningTokens {
		o.responseTokens = generated - o.reasoningTokens
	}
	plain := formatLiveMetricsBreakdownPlain(o.model, stage, prefillRate, decodeRate, o.reasoningTokens, o.responseTokens, contextPercent)
	o.render(formatFooter(plain))
	if o.notify != nil {
		o.notify(overlayUpdate{model: o.model, footer: plain, slots: slots})
	}
	o.lastAt, o.lastPrompt, o.lastGenerated, o.hasMeasurement = now, prompt, generated, true
}

// Complete freezes the final rates and token count and marks a successful
// response as complete until the next turn begins producing slot updates.
func (o *liveMetricsOverlay) Complete() {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	var decodeRate *float64
	if o.lastDecodeRate > 0 {
		decodeRate = &o.lastDecodeRate
	}
	plain := formatLiveMetricsBreakdownPlain(o.model, "Complete", o.lastPrefillRate, decodeRate, o.reasoningTokens, o.responseTokens, o.lastContextPercent)
	o.render(formatFooter(plain))
	if o.notify != nil {
		o.notify(overlayUpdate{model: o.model, footer: plain})
	}
}

// UpdateLoading is an exported operation provided by this package.
func (o *liveMetricsOverlay) UpdateLoading(progress modelLoadProgress) {
	o.updateLoadingForModel("", progress)
}

// updateLoadingForModel performs the internal operation described by its signature.
func (o *liveMetricsOverlay) updateLoadingForModel(model string, progress modelLoadProgress) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if model != "" && o.model != model || o.stopped {
		return
	}

	o.loading = true
	plain := formatModelLoadingPlain(o.model, progress)
	o.render(formatFooter(plain))
	if o.notify != nil {
		o.notify(overlayUpdate{model: o.model, footer: plain})
	}
}

// StartModelLoad switches the overlay to a new model and displays its loading
// state before the model-loading monitor begins reporting progress.
func (o *liveMetricsOverlay) StartModelLoad(model string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.model = model
	o.loading = true
	o.startedAt = time.Now()
	o.hasMeasurement = false
	o.lastDecodeRate = 0
	o.lastPrefillRate = 0
	o.lastGenerated = 0
	o.reasoningTokens = 0
	o.responseTokens = 0
	o.reasoningPhase = false
	o.lastContextPercent = 0
	o.render(formatFooter(formatModelLoadingPlain(model, modelLoadProgress{})))
	if o.notify != nil {
		o.notify(overlayUpdate{model: model, footer: formatModelLoadingPlain(model, modelLoadProgress{})})
	}
	o.mu.Unlock()
}

// SetModel is an exported operation provided by this package.
func (o *liveMetricsOverlay) SetModel(model string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.model = model
	o.mu.Unlock()
}

// SetReasoningPhase lets the stream parser split the existing decoded-token
// metric between reasoning and final-answer generation.
func (o *liveMetricsOverlay) SetReasoningPhase(reasoning bool) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.reasoningPhase = reasoning
	if !reasoning && o.lastGenerated >= o.reasoningTokens {
		o.responseTokens = o.lastGenerated - o.reasoningTokens
	}
	if o.hasMeasurement {
		var decodeRate *float64
		if o.lastDecodeRate > 0 {
			decodeRate = &o.lastDecodeRate
		}
		o.render(formatLiveMetricsBreakdownPlain(o.model, "Decode", o.lastPrefillRate, decodeRate, o.reasoningTokens, o.responseTokens, o.lastContextPercent))
	}
}

// ModelLoaded is an exported operation provided by this package.
func (o *liveMetricsOverlay) ModelLoaded() {
	o.modelLoadedForModel("")
}

// StartInference clears the model-ready state before a request begins. A
// model can be ready while an inference is already running, so leaving the
// ready label in place makes the live overlay lie until the next slot sample.
func (o *liveMetricsOverlay) StartInference(model string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if model != "" && o.model != model || o.stopped {
		return
	}
	o.loading = false
	o.startedAt = time.Now()
	o.hasMeasurement = false
	o.lastDecodeRate = 0
	o.lastPrefillRate = 0
	o.lastGenerated = 0
	o.reasoningTokens = 0
	o.responseTokens = 0
	o.reasoningPhase = false
	o.lastContextPercent = 0
	plain := formatLiveMetricsPlain(o.model, "Prefill", 0, nil, 0, 0)
	o.render(formatFooter(plain))
	if o.notify != nil {
		o.notify(overlayUpdate{model: o.model, footer: plain})
	}
}

// modelLoadedForModel performs the internal operation described by its signature.
func (o *liveMetricsOverlay) modelLoadedForModel(model string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if model != "" && o.model != model || o.stopped {
		return
	}

	o.loading = false
	o.startedAt = time.Now()
	o.hasMeasurement = false
	o.lastDecodeRate = 0
	o.lastPrefillRate = 0
	o.lastGenerated = 0
	o.reasoningTokens = 0
	o.responseTokens = 0
	o.reasoningPhase = false
	o.lastContextPercent = 0
	plain := formatModelReadyPlain(o.model)
	o.render(formatFooter(plain))
	if o.notify != nil {
		o.notify(overlayUpdate{model: o.model, footer: plain, modelReady: true})
	}
}

// formatModelReadyPlain performs the internal operation described by its signature.
func formatModelReadyPlain(model string) string {
	return fmt.Sprintf("  [Induction: Live Metrics] %s | Ready ", model)
}

// render performs the internal operation described by its signature.
func (o *liveMetricsOverlay) render(content string) {
	if o.footer != nil {
		if o.mcp != "" {
			o.footer.UpdateRows(formatMCPFooter(o.mcp), content)
		} else {
			o.footer.Update(content)
		}
		return
	}
	if o.area != nil {
		o.area.Update(content)
	}
}

// UpdateMCP is an exported operation provided by this package.
func (o *liveMetricsOverlay) UpdateMCP(content string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.mcp = content
	var decodeRate *float64
	if o.lastDecodeRate > 0 {
		decodeRate = &o.lastDecodeRate
	}
	metrics := formatFooter(formatLiveMetricsPlain(o.model, "MCP", o.lastPrefillRate, decodeRate, o.lastGenerated, o.lastContextPercent))
	o.render(metrics)
}

// Stop is an exported operation provided by this package.
func (o *liveMetricsOverlay) Stop() {
	if o == nil {
		return
	}
	o.mu.Lock()
	if o.stopped {
		o.mu.Unlock()
		return
	}
	o.stopped = true
	if o.area != nil {
		_ = o.area.Stop()
	}
	if o.footer != nil {
		o.footer.Stop()
	}
	o.mu.Unlock()
	unregisterLiveMetricsOverlay(o)
}

// formatLiveMetrics performs the internal operation described by its signature.
func formatLiveMetrics(model, stage string, promptRate float64, decodeRate *float64, tokensGenerated, contextPercent float64) string {
	return formatFooter(formatLiveMetricsPlain(model, stage, promptRate, decodeRate, tokensGenerated, contextPercent))
}

// formatLiveMetricsPlain performs the internal operation described by its signature.
func formatLiveMetricsPlain(model, stage string, promptRate float64, decodeRate *float64, tokensGenerated, contextPercent float64) string {
	return formatLiveMetricsBreakdownPlain(model, stage, promptRate, decodeRate, 0, tokensGenerated, contextPercent)
}

// formatLiveMetricsBreakdownPlain performs the internal operation described by its signature.
func formatLiveMetricsBreakdownPlain(model, stage string, promptRate float64, decodeRate *float64, reasoningTokens, responseTokens, contextPercent float64) string {
	decode := "n/a"
	if decodeRate != nil {
		decode = fmt.Sprintf("%.1f", *decodeRate)
	}
	content := fmt.Sprintf(
		"  [Induction: Live Metrics] %s | Stage: %s | Prefill (tok/s): %.1f  |  Decode (tok/s): %s  |  Reasoning Tokens: %.0f | Decoded Tokens: %.0f  |  Ctx Used: %.1f%% ",
		model, stage, promptRate, decode, reasoningTokens, responseTokens, contextPercent,
	)

	return content
}

// formatModelLoading performs the internal operation described by its signature.
func formatModelLoading(model string, progress modelLoadProgress) string {
	return formatFooter(formatModelLoadingPlain(model, progress))
}

// formatModelLoadingPlain performs the internal operation described by its signature.
func formatModelLoadingPlain(model string, progress modelLoadProgress) string {
	stage := loadingStageLabel(progress.Current)
	if stage == "" {
		stage = loadingStageLabel(progress.Stage)
	}
	if stage == "" {
		stage = "Model"
	}

	stagePosition := ""
	if len(progress.Stages) > 0 {
		for i, candidate := range progress.Stages {
			if candidate == progress.Current || candidate == progress.Stage {
				stagePosition = fmt.Sprintf("  |  Stage %d/%d", i+1, len(progress.Stages))
				break
			}
		}
	}
	percent := ""
	if progress.Value != nil {
		percent = fmt.Sprintf("  |  %.1f%%", *progress.Value*100)
	}

	return fmt.Sprintf("  [Induction: Live Metrics] %s | Loading: %s%s%s ", model, stage, stagePosition, percent)
}

// loadingStageLabel performs the internal operation described by its signature.
func loadingStageLabel(stage string) string {
	switch stage {
	case "text_model":
		return "Text model"
	case "spec_model":
		return "Speculative model"
	case "mmproj_model":
		return "Multimodal projector"
	default:
		return stage
	}
}

// formatFooter performs the internal operation described by its signature.
func formatFooter(content string) string {
	return defaultConsoleUITheme.renderFooterBar(DefaultUnicode.DoubleArrowRight, content, false)
}

// formatMCPFooter performs the internal operation described by its signature.
func formatMCPFooter(content string) string {
	return defaultConsoleUITheme.renderFooterBar(DefaultUnicode.DoubleArrowRight, content, true)
}

// activeSlotMetrics performs the internal operation described by its signature.
func activeSlotMetrics(slots SlotsData) (prompt, generated, used, capacity float64, ok bool) {
	for _, slot := range slots {
		if processing, exists := slot["is_processing"].(bool); exists && !processing {
			continue
		}
		capacity, _ = number(slot["n_ctx"])
		used, _ = number(slot["n_prompt_tokens"])
		prompt, _ = number(slot["n_prompt_tokens_processed"])
		generated, _ = number(slot["n_decoded"])
		if next, exists := slot["next_token"].([]interface{}); exists && len(next) > 0 {
			if values, valid := next[0].(map[string]interface{}); valid {
				if decoded, valid := number(values["n_decoded"]); valid {
					generated = decoded
				}
			}
		}
		if used == 0 {
			used = prompt + generated
		}
		return prompt, generated, used, capacity, capacity > 0
	}
	return 0, 0, 0, 0, false
}

// number performs the internal operation described by its signature.
func number(value interface{}) (float64, bool) {
	switch value := value.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case jsonNumber:
		parsed, err := value.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

// jsonNumber is the small part of encoding/json.Number needed here and keeps
// number usable with callers that decode JSON using UseNumber.
// jsonNumber is an internal data type used by this package.
type jsonNumber interface {
	Float64() (float64, error)
}

// nonNegative performs the internal operation described by its signature.
func nonNegative(value float64) float64 {
	if value < 0 {
		return 0
	}
	return value
}
