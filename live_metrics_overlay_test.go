package induction

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestActiveSlotMetrics(t *testing.T) {
	slots := SlotsData{
		{"is_processing": false, "n_ctx": float64(1)},
		{
			"is_processing":             true,
			"n_ctx":                     float64(4096),
			"n_prompt_tokens":           float64(1024),
			"n_prompt_tokens_processed": float64(128),
			"next_token": []interface{}{
				map[string]interface{}{"n_decoded": float64(256)},
			},
		},
	}

	prompt, generated, used, capacity, ok := activeSlotMetrics(slots)
	if !ok || prompt != 128 || generated != 256 || used != 1024 || capacity != 4096 {
		t.Fatalf("unexpected metrics: prompt=%v generated=%v used=%v capacity=%v ok=%v", prompt, generated, used, capacity, ok)
	}
}

func TestActiveSlotMetricsFallsBackToTokenSum(t *testing.T) {
	slots := SlotsData{{
		"n_ctx":                     float64(1000),
		"n_prompt_tokens_processed": float64(100),
		"n_decoded":                 float64(50),
	}}

	_, _, used, _, ok := activeSlotMetrics(slots)
	if !ok || used != 150 {
		t.Fatalf("expected 150 used tokens, got %v", used)
	}
}

func TestFormatLiveMetrics(t *testing.T) {
	decodeRate := 56.78
	rendered := formatLiveMetrics("model-id", "Decode", 12.34, &decodeRate, 1234, 25)
	if strings.Contains(rendered, "\n") {
		t.Fatalf("expected a single-line footer: %q", rendered)
	}
	for _, expected := range []string{"[Induction: Live Metrics] model-id | Stage: Decode", "Prefill (tok/s): 12.3", "Decode (tok/s): 56.8", "Tokens Generated: 1234", "Context Used: 25.0%"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected rendered overlay to contain %q: %q", expected, rendered)
		}
	}
	prefill := formatLiveMetrics("model-id", "Prefill", 12.34, nil, 0, 25)
	if !strings.Contains(prefill, "Decode (tok/s): n/a") {
		t.Fatalf("expected prefill decode rate to be unavailable: %q", prefill)
	}
}

func TestDecodeRateDoesNotRevertToZero(t *testing.T) {
	var output bytes.Buffer
	overlay := &liveMetricsOverlay{
		footer:    &stickyFooter{writer: &output, size: func() (int, int, error) { return 120, 24, nil }, width: 120, height: 24, active: true},
		startedAt: time.Now().Add(-time.Second),
		model:     "model-id",
	}
	overlay.Update(SlotsData{{"n_ctx": 100, "n_prompt_tokens_processed": 10, "n_decoded": 5}})
	retained := overlay.lastDecodeRate
	if retained <= 0 {
		t.Fatalf("expected a positive decode rate, got %v", retained)
	}
	overlay.lastAt = time.Now().Add(-time.Second)
	overlay.Update(SlotsData{{"n_ctx": 100, "n_prompt_tokens_processed": 10, "n_decoded": 5}})
	if overlay.lastDecodeRate != retained {
		t.Fatalf("decode rate changed from %v to %v without new decoded tokens", retained, overlay.lastDecodeRate)
	}
	if !strings.Contains(output.String(), fmt.Sprintf("Decode (tok/s): %.1f", retained)) {
		t.Fatalf("expected retained decode rate in output: %q", output.String())
	}
}

func TestPrefillRatePersistsThroughDecodeAndCompletion(t *testing.T) {
	var updates []overlayUpdate
	overlay := &liveMetricsOverlay{
		startedAt: time.Now().Add(-time.Second),
		model:     "model-id",
		notify:    func(update overlayUpdate) { updates = append(updates, update) },
	}
	overlay.Update(SlotsData{{"n_ctx": 100, "n_prompt_tokens_processed": 10, "n_decoded": 0}})
	retained := overlay.lastPrefillRate
	if retained <= 0 {
		t.Fatalf("expected positive prefill rate, got %v", retained)
	}
	overlay.lastAt = time.Now().Add(-time.Second)
	overlay.Update(SlotsData{{"n_ctx": 100, "n_prompt_tokens_processed": 1000, "n_decoded": 25}})
	if overlay.lastPrefillRate != retained {
		t.Fatalf("prefill rate changed during decode: got %v want %v", overlay.lastPrefillRate, retained)
	}
	overlay.Complete()
	footer := updates[len(updates)-1].footer
	for _, expected := range []string{
		"Stage: Complete",
		fmt.Sprintf("Prefill (tok/s): %.1f", retained),
		"Tokens Generated: 25",
	} {
		if !strings.Contains(footer, expected) {
			t.Fatalf("completed footer does not contain %q: %q", expected, footer)
		}
	}
}

func TestFormatModelLoading(t *testing.T) {
	value := 0.5036
	rendered := formatModelLoading("model-id", modelLoadProgress{
		Stages:  []string{"text_model", "spec_model", "mmproj_model"},
		Current: "text_model",
		Value:   &value,
	})
	if strings.Contains(rendered, "\n") {
		t.Fatalf("expected a single-line loading footer: %q", rendered)
	}
	for _, expected := range []string{"[Induction: Live Metrics] model-id | Loading: Text model", "Stage 1/3", "50.4%"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected loading footer to contain %q: %q", expected, rendered)
		}
	}
}

func TestFormatModelLoadingStageOnly(t *testing.T) {
	rendered := formatModelLoading("model-id", modelLoadProgress{Stage: "mmproj_model"})
	if !strings.Contains(rendered, "Loading: Multimodal projector") {
		t.Fatalf("expected stage-only transition to be rendered: %q", rendered)
	}
}
