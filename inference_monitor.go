package induction

import (
	"context"
	"sync"
	"time"
)

// inferenceMonitor owns the optional live overlay and slot sampling performed
// during one inference request.
type inferenceMonitor struct {
	mu          sync.RWMutex
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	overlay     *liveMetricsOverlay
	slots       SlotsData
	ownsOverlay bool
	slotsReady  chan struct{}
	readyOnce   sync.Once
}

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

func (c *Client) startInferenceMonitorWithOverlay(ctx context.Context, model string, collectSamples bool, supplied *liveMetricsOverlay) *inferenceMonitor {
	monitorCtx, cancel := context.WithCancel(ctx)
	monitor := &inferenceMonitor{cancel: cancel, overlay: supplied}
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
	// Streaming snapshot clients are intentionally created without their own
	// overlay, but still need to update the overlay owned by this monitor when
	// the stream enters or leaves a reasoning block.
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
			monitor.slots = c.pollSlots(monitorCtx, model, monitor.overlay, func(slots SlotsData) {
				monitor.mu.Lock()
				monitor.slots = slots
				monitor.mu.Unlock()
			})
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

func (m *inferenceMonitor) markModelReady() {
	if m == nil || m.slotsReady == nil {
		return
	}
	m.readyOnce.Do(func() { close(m.slotsReady) })
}

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
