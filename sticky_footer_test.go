package induction

import (
	"bytes"
	"strings"
	"testing"
)

func TestStickyFooterReservesUpdatesAndRestoresTerminal(t *testing.T) {
	var output bytes.Buffer
	footer := &stickyFooter{
		writer: &output,
		size:   func() (int, int, error) { return 80, 24, nil },
		width:  80,
		height: 24,
		active: true,
	}
	footer.reserve(80, 24)
	footer.Update("metrics")
	footer.Stop()

	rendered := output.String()
	for _, expected := range []string{
		"\x1b[1;23r", // reserve all but the final row
		"\x1b[24;1H", // draw on the physical final row
		"\x1b[2Kmetrics",
		ansiResetScrollRegion,
		ansiShowCursor,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected terminal sequence %q in %q", expected, rendered)
		}
	}
	if footer.active {
		t.Fatal("expected footer to be inactive after Stop")
	}
}

func TestStickyFooterReconfiguresAfterResize(t *testing.T) {
	var output bytes.Buffer
	width, height := 80, 24
	footer := &stickyFooter{
		writer: &output,
		size:   func() (int, int, error) { return width, height, nil },
		width:  width,
		height: height,
		active: true,
	}

	width, height = 100, 40
	footer.Update("resized")
	rendered := output.String()
	if !strings.Contains(rendered, "\x1b[1;39r") || !strings.Contains(rendered, "\x1b[40;1H") {
		t.Fatalf("expected resized footer geometry, got %q", rendered)
	}
}

func TestStickyFooterStopIsIdempotent(t *testing.T) {
	var output bytes.Buffer
	footer := &stickyFooter{
		writer: &output,
		size:   func() (int, int, error) { return 80, 24, nil },
		width:  80,
		height: 24,
		active: true,
	}
	footer.Stop()
	first := output.String()
	footer.Stop()
	if output.String() != first {
		t.Fatal("expected repeated Stop to produce no additional terminal output")
	}
}

func TestStickyFooterRendersTwoRowsAtomically(t *testing.T) {
	var output bytes.Buffer
	footer := &stickyFooter{
		writer: &output,
		size:   func() (int, int, error) { return 80, 24, nil },
		width:  80,
		height: 24,
		active: true,
		rows:   2,
	}
	footer.reserve(80, 24)
	footer.UpdateRows("mcp", "metrics")
	footer.Stop()

	rendered := output.String()
	for _, expected := range []string{
		"\x1b[1;22r",
		"\x1b[23;1H\x1b[2Kmcp",
		"\x1b[24;1H\x1b[2Kmetrics",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected two-row terminal sequence %q in %q", expected, rendered)
		}
	}
}
