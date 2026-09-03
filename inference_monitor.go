package induction

import (
	"context"
	"sync"
	"time"
)

// inferenceMonitor owns the optional live overlay and slot sampling performed
// during one inference request.
type inferenceMonitor struct {
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	overlay     *liveMetricsOverlay
	slots       SlotsData
	ownsOverlay bool
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
	if monitor.overlay == nil && c.opts.liveMetricsOverlay != nil {
		monitor.overlay = c.opts.liveMetricsOverlay
	}
	if monitor.overlay == nil && c.opts.enableLiveMetricsOverlay {
		monitor.overlay = startLiveMetricsOverlay(model)
		monitor.ownsOverlay = monitor.overlay != nil
	}

	if collectSamples || monitor.overlay != nil {
		monitor.wg.Add(1)
		go func() {
			defer monitor.wg.Done()
			monitor.slots = c.pollSlots(monitorCtx, model, monitor.overlay)
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
	if removeOverlay && m.overlay != nil && m.ownsOverlay {
		m.overlay.Stop()
	}
	return m.slots
}
