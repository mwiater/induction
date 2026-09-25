package induction

import (
	"bytes"
	"strings"
	"testing"
)

func TestCleanupRemovesLiveMetricsOverlay(t *testing.T) {
	var terminal bytes.Buffer
	footer := &stickyFooter{
		writer: &terminal,
		size:   func() (int, int, error) { return 80, 24, nil },
		width:  80,
		height: 24,
		active: true,
	}
	overlay := &liveMetricsOverlay{footer: footer}
	registerLiveMetricsOverlay(overlay)

	var output bytes.Buffer
	Cleanup(&output)
	if footer.active || !overlay.stopped {
		t.Fatal("expected cleanup to stop the live overlay")
	}
	want := "Runnnig: Cleanup Hook\nRemoving: Live Metrics Overlay\nCleanup: Complete\n"
	if output.String() != want {
		t.Fatalf("cleanup output = %q, want %q", output.String(), want)
	}
	if !strings.Contains(terminal.String(), ansiClearLine) {
		t.Fatalf("expected terminal row to be cleared: %q", terminal.String())
	}
}

func TestCleanupWithoutOverlay(t *testing.T) {
	_ = takeLiveMetricsOverlays()
	var output bytes.Buffer
	Cleanup(&output)
	want := "Runnnig: Cleanup Hook\nCleanup: Complete\n"
	if output.String() != want {
		t.Fatalf("cleanup output = %q, want %q", output.String(), want)
	}
}
