// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"context"
	"sync"
	"time"
)

// inferenceMonitor owns the optional live overlay and slot sampling performed
// during one inference request.
// inferenceMonitor is an internal data type used by this package.
type inferenceMonitor struct {
	// mu stores the value represented by this member.
	mu sync.RWMutex
	// cancel stores the value represented by this member.
	cancel context.CancelFunc
	// wg stores the value represented by this member.
	wg sync.WaitGroup
	// overlay stores the value represented by this member.
	overlay *liveMetricsOverlay
	// slots stores the value represented by this member.
	slots SlotsData
	// ownsOverlay stores the value represented by this member.
	ownsOverlay bool
	// slotsReady stores the value represented by this member.
	slotsReady        chan struct{}
	contextTokens     int
	contextCapacity   int
	contextPeakTokens int
	contextShifts     int
	lastContextTokens int
	hasContext        bool
	startedAt         time.Time
	slotSamples       []SnapshotSlotSample
	// readyOnce stores the value represented by this member.
	readyOnce sync.Once
}

func (m *inferenceMonitor) recordSlots(slots SlotsData) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.slots = slots
	if len(slots) == 0 {
		return
	}
	activeSlots := 0
	activeSlotID := 0
	selected := slots[0]
	for _, slot := range slots {
		processing, _ := slot["is_processing"].(bool)
		if !processing {
			continue
		}
		selected = slot
		activeSlots++
		if value, ok := number(slot["id"]); ok {
			activeSlotID = int(value)
		}
	}
	used, _ := number(selected["n_prompt_tokens"])
	capacity, _ := number(selected["n_ctx"])
	prompt, _ := number(selected["n_prompt_tokens_processed"])
	generated, _ := slotDecodedTokens(selected)
	if used == 0 {
		used = prompt + generated
	}
	if prompt == 0 {
		prompt = used
	}
	cached, _ := number(selected["n_prompt_tokens_cache"])
	processing, _ := selected["is_processing"].(bool)
	atMS := time.Since(m.startedAt).Milliseconds()
	m.slotSamples = append(m.slotSamples, SnapshotSlotSample{AtMS: atMS, SlotID: activeSlotID, ActiveSlots: activeSlots, ContextTokens: int(used), ContextCapacity: int(capacity), PromptTokens: int(used), PromptTokensProcessed: int(prompt), DecodedTokens: int(generated), CachedTokens: int(cached), IsProcessing: processing})
	if capacity <= 0 {
		return
	}
	if m.hasContext && int(used) < m.lastContextTokens {
		m.contextShifts++
	}
	m.contextTokens = int(used)
	m.contextCapacity = int(capacity)
	if !m.hasContext || int(used) > m.contextPeakTokens {
		m.contextPeakTokens = int(used)
	}
	m.lastContextTokens = int(used)
	m.hasContext = true
}

// slotDecodedTokens supports both the legacy top-level field and the current
// llama-server shape, where n_decoded is nested under next_token.
func slotDecodedTokens(slot map[string]interface{}) (float64, bool) {
	if value, ok := number(slot["n_decoded"]); ok {
		return value, true
	}
	if next, ok := slot["next_token"].([]interface{}); ok && len(next) > 0 {
		if values, ok := next[0].(map[string]interface{}); ok {
			return number(values["n_decoded"])
		}
	}
	return 0, false
}

func (m *inferenceMonitor) slotSamplesSnapshot() []SnapshotSlotSample {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]SnapshotSlotSample(nil), m.slotSamples...)
}

func (m *inferenceMonitor) contextTelemetry() *SnapshotContextTelemetry {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.hasContext || m.contextCapacity <= 0 {
		return nil
	}
	tokens, capacity, peak, shifts := m.contextTokens, m.contextCapacity, m.contextPeakTokens, m.contextShifts
	percent := float64(tokens) / float64(capacity) * 100
	peakPercent := float64(peak) / float64(capacity) * 100
	return &SnapshotContextTelemetry{Tokens: &tokens, Capacity: &capacity, Percent: &percent, PeakTokens: &peak, PeakPercent: &peakPercent, Shifts: &shifts}
}

// latestMetrics performs the internal operation described by its signature.
func (m *inferenceMonitor) latestMetrics() (generated, used, capacity int, ok bool) {
	if m == nil {
		return 0, 0, 0, false
	}
	m.mu.RLock()
	slots := m.slots
	m.mu.RUnlock()
	_, generatedValue, usedValue, capacityValue, ok := activeSlotMetrics(slots)
	return int(generatedValue), int(usedValue), int(capacityValue), ok
}

// startInferenceMonitor begins monitoring before inference so model-loading
// events are not missed. Snapshot requests retain the newest slots payload even
// when the live overlay is disabled; other requests poll only for the overlay.
func (c *Client) startInferenceMonitor(ctx context.Context, model string, collectSamples bool) *inferenceMonitor {
	return c.startInferenceMonitorWithOverlay(ctx, model, collectSamples, nil)
}

// startInferenceMonitorWithOverlay performs the internal operation described by its signature.
// Additional context: Streaming snapshot clients are intentionally created without their own overlay, but still need to update the overlay owned by this monitor when the stream enters or leaves a reasoning block.
func (c *Client) startInferenceMonitorWithOverlay(ctx context.Context, model string, collectSamples bool, supplied *liveMetricsOverlay) *inferenceMonitor {
	monitorCtx, cancel := context.WithCancel(ctx)
	monitor := &inferenceMonitor{cancel: cancel, overlay: supplied, startedAt: time.Now()}
	if supplied != nil {
		monitor.slotsReady = make(chan struct{})
	}
	if monitor.overlay == nil && c.opts.liveMetricsOverlay != nil {
		monitor.overlay = c.opts.liveMetricsOverlay
	}
	if monitor.overlay == nil && c.opts.liveMetricsOverlayEnabled {
		monitor.overlay = startLiveMetricsOverlay(model)
		monitor.ownsOverlay = monitor.overlay != nil
	}

	if monitor.overlay != nil && c.opts.progressOverlay == nil {
		c.opts.progressOverlay = monitor.overlay
	}

	if collectSamples || monitor.overlay != nil {
		monitor.wg.Add(1)
		go func() {
			defer monitor.wg.Done()
			if monitor.slotsReady != nil {
				select {
				case <-monitor.slotsReady:
				case <-monitorCtx.Done():
					return
				}
			}
			monitor.recordSlots(c.pollSlots(monitorCtx, model, monitor.overlay, func(slots SlotsData) {
				monitor.recordSlots(slots)
			}))
		}()
	}

	if monitor.overlay != nil {
		ready := make(chan struct{})
		monitor.wg.Add(1)
		go func() {
			defer monitor.wg.Done()
			if err := c.monitorModelLoading(monitorCtx, model, monitor.overlay, ready); err != nil && monitorCtx.Err() == nil {
				c.logf("overlay: model loading stream failed: %v", err)
			}
		}()
		waitForMonitorReady(ctx, ready)
	}

	return monitor
}

// markModelReady performs the internal operation described by its signature.
func (m *inferenceMonitor) markModelReady() {
	if m == nil || m.slotsReady == nil {
		return
	}
	m.readyOnce.Do(func() { close(m.slotsReady) })
}

// waitForMonitorReady performs the internal operation described by its signature.
func waitForMonitorReady(ctx context.Context, ready <-chan struct{}) {
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ready:
	case <-timer.C:
	case <-ctx.Done():
	}
}

// Stop ends all monitoring, waits for its goroutines, removes the overlay, and
// returns the newest slots payload collected for a snapshot.
func (m *inferenceMonitor) Stop() SlotsData {
	return m.stop(true)
}

// stopKeepingOverlay ends background monitoring while leaving the last
// rendered overlay visible for application-level cleanup.
func (m *inferenceMonitor) stopKeepingOverlay() SlotsData {
	return m.stop(false)
}

// stop performs the internal operation described by its signature.
func (m *inferenceMonitor) stop(removeOverlay bool) SlotsData {
	if m == nil {
		return nil
	}
	m.cancel()
	m.wg.Wait()
	m.mu.RLock()
	slots := m.slots
	m.mu.RUnlock()
	if removeOverlay && m.overlay != nil && m.ownsOverlay {
		m.overlay.Stop()
	}
	return slots
}
