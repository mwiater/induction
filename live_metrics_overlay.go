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
type liveMetricsOverlay struct {
	mu                 sync.Mutex
	area               *pterm.AreaPrinter
	footer             *stickyFooter
	startedAt          time.Time
	lastAt             time.Time
	lastPrompt         float64
	lastGenerated      float64
	lastPrefillRate    float64
	lastDecodeRate     float64
	lastContextPercent float64
	hasMeasurement     bool
	loading            bool
	model              string
	stopped            bool
	mcp                string
	// notify mirrors plain, structured terminal updates to a Bubble Tea program.
	// The legacy footer remains available for non-chat inference entry points.
	notify func(overlayUpdate)
}

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

type overlayUpdate struct {
	footer     string
	slots      SlotsData
	modelReady bool
}

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

func (o *liveMetricsOverlay) Update(slots SlotsData) {
	prompt, generated, used, capacity, ok := activeSlotMetrics(slots)
	if !ok {
		// The sidebar still needs idle slot details even when no slot can produce
		// an active-inference rate measurement.
		if o.notify != nil {
			o.notify(overlayUpdate{slots: slots})
		}
		return
	}

	o.mu.Lock()
	defer o.mu.Unlock()
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
	plain := formatLiveMetricsPlain(o.model, stage, prefillRate, decodeRate, generated, contextPercent)
	o.render(formatFooter(plain))
	if o.notify != nil {
		o.notify(overlayUpdate{footer: plain, slots: slots})
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
	plain := formatLiveMetricsPlain(o.model, "Complete", o.lastPrefillRate, decodeRate, o.lastGenerated, o.lastContextPercent)
	o.render(formatFooter(plain))
	if o.notify != nil {
		o.notify(overlayUpdate{footer: plain})
	}
}

func (o *liveMetricsOverlay) UpdateLoading(progress modelLoadProgress) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.loading = true
	plain := formatModelLoadingPlain(o.model, progress)
	o.render(formatFooter(plain))
	if o.notify != nil {
		o.notify(overlayUpdate{footer: plain})
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
	o.lastContextPercent = 0
	o.render(formatFooter(formatModelLoadingPlain(model, modelLoadProgress{})))
	if o.notify != nil {
		o.notify(overlayUpdate{footer: formatModelLoadingPlain(model, modelLoadProgress{})})
	}
	o.mu.Unlock()
}

func (o *liveMetricsOverlay) SetModel(model string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.model = model
	o.mu.Unlock()
}

func (o *liveMetricsOverlay) ModelLoaded() {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.loading = false
	o.startedAt = time.Now()
	o.hasMeasurement = false
	o.lastDecodeRate = 0
	o.lastPrefillRate = 0
	o.lastGenerated = 0
	o.lastContextPercent = 0
	plain := formatModelReadyPlain(o.model)
	o.render(formatFooter(plain))
	if o.notify != nil {
		o.notify(overlayUpdate{footer: plain, modelReady: true})
	}
}

func formatModelReadyPlain(model string) string {
	return fmt.Sprintf("  [Induction: Live Metrics] %s | Ready ", model)
}

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

func formatLiveMetrics(model, stage string, promptRate float64, decodeRate *float64, tokensGenerated, contextPercent float64) string {
	return formatFooter(formatLiveMetricsPlain(model, stage, promptRate, decodeRate, tokensGenerated, contextPercent))
}

func formatLiveMetricsPlain(model, stage string, promptRate float64, decodeRate *float64, tokensGenerated, contextPercent float64) string {
	decode := "n/a"
	if decodeRate != nil {
		decode = fmt.Sprintf("%.1f", *decodeRate)
	}
	content := fmt.Sprintf(
		"  [Induction: Live Metrics] %s | Stage: %s | Prefill (tok/s): %.1f  |  Decode (tok/s): %s  |  Tokens Generated: %.0f  |  Context Used: %.1f%% ",
		model, stage, promptRate, decode, tokensGenerated, contextPercent,
	)

	return content
}

func formatModelLoading(model string, progress modelLoadProgress) string {
	return formatFooter(formatModelLoadingPlain(model, progress))
}

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

func formatFooter(content string) string {
	return defaultConsoleUITheme.renderFooterBar(DefaultUnicode.DoubleArrowRight, content, false)
}

func formatMCPFooter(content string) string {
	return defaultConsoleUITheme.renderFooterBar(DefaultUnicode.DoubleArrowRight, content, true)
}

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
type jsonNumber interface {
	Float64() (float64, error)
}

func nonNegative(value float64) float64 {
	if value < 0 {
		return 0
	}
	return value
}
