package induction

import (
	"fmt"
	"io"
	"sync"
)

var liveOverlays = struct {
	sync.Mutex
	items map[*liveMetricsOverlay]struct{}
}{items: make(map[*liveMetricsOverlay]struct{})}

// Cleanup removes any live metrics overlays and prints the application cleanup
// status. Applications should call Cleanup before fatal exits because
// Cleanup stops active terminal overlays and restores normal terminal output.
// It is safe to call even when no overlay is active.
func Cleanup(out io.Writer) {
	if out == nil {
		out = io.Discard
	}

	overlays := takeLiveMetricsOverlays()
	for _, overlay := range overlays {
		overlay.Stop()
	}

	_, _ = fmt.Fprintln(out, "Runnnig: Cleanup Hook")
	if len(overlays) > 0 {
		_, _ = fmt.Fprintln(out, "Removing: Live Metrics Overlay")
	}
	_, _ = fmt.Fprintln(out, "Cleanup: Complete")
}

func registerLiveMetricsOverlay(overlay *liveMetricsOverlay) {
	if overlay == nil || (overlay.footer == nil && overlay.area == nil) {
		return
	}
	liveOverlays.Lock()
	liveOverlays.items[overlay] = struct{}{}
	liveOverlays.Unlock()
}

func unregisterLiveMetricsOverlay(overlay *liveMetricsOverlay) {
	liveOverlays.Lock()
	delete(liveOverlays.items, overlay)
	liveOverlays.Unlock()
}

func takeLiveMetricsOverlays() []*liveMetricsOverlay {
	liveOverlays.Lock()
	overlays := make([]*liveMetricsOverlay, 0, len(liveOverlays.items))
	for overlay := range liveOverlays.items {
		overlays = append(overlays, overlay)
	}
	clear(liveOverlays.items)
	liveOverlays.Unlock()
	return overlays
}
