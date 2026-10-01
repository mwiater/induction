package induction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestReasoningCutoffValidation(t *testing.T) {
	one := 1
	zero := 0
	seconds := 2.5
	percent := 70.0
	cases := []struct {
		name    string
		cfg     *ReasoningCutoffConfig
		wantErr bool
	}{
		{"disabled", &ReasoningCutoffConfig{}, false},
		{"valid", &ReasoningCutoffConfig{Enabled: true, MaxTokens: &one, MaxSeconds: &seconds, MaxContextPercent: &percent}, false},
		{"tokens only", &ReasoningCutoffConfig{Enabled: true, MaxTokens: &one}, false},
		{"seconds only", &ReasoningCutoffConfig{Enabled: true, MaxSeconds: &seconds}, false},
		{"context only", &ReasoningCutoffConfig{Enabled: true, MaxContextPercent: &percent}, false},
		{"missing threshold", &ReasoningCutoffConfig{Enabled: true}, true},
		{"zero tokens", &ReasoningCutoffConfig{Enabled: true, MaxTokens: &zero}, true},
		{"zero seconds", &ReasoningCutoffConfig{Enabled: true, MaxSeconds: float64Ptr(0)}, true},
		{"zero context", &ReasoningCutoffConfig{Enabled: true, MaxContextPercent: float64Ptr(0)}, true},
		{"over context", &ReasoningCutoffConfig{Enabled: true, MaxContextPercent: float64Ptr(100.1)}, true},
		{"disabled invalid threshold", &ReasoningCutoffConfig{MaxTokens: &zero}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.validate("reasoning.cutoff"); (got != nil) != tc.wantErr {
				t.Fatalf("error=%v wantErr=%v", got, tc.wantErr)
			}
		})
	}
}

func float64Ptr(value float64) *float64 { return &value }

func TestReasoningWatchdogFirstThresholdWins(t *testing.T) {
	tokens, seconds := 2, 60.0
	calls := 0
	w := newReasoningWatchdog(&ReasoningCutoffConfig{Enabled: true, MaxTokens: &tokens, MaxSeconds: &seconds}, func(string) (*ChatCompletionControlResponse, error) {
		calls++
		return &ChatCompletionControlResponse{Success: true}, nil
	})
	w.setID("chatcmpl-test")
	w.start()
	w.update(2, nil, nil)
	w.update(99, nil, nil)
	s := w.snapshotCopy()
	if calls != 1 || !s.Triggered || s.Reason != ReasoningCutoffReasonMaxTokens || !s.ControlSuccess {
		t.Fatalf("calls=%d snapshot=%+v", calls, s)
	}
}

func TestReasoningWatchdogTimeOnlyCutoff(t *testing.T) {
	controlCalled := make(chan string, 1)
	seconds := 0.01
	w := newReasoningWatchdog(&ReasoningCutoffConfig{Enabled: true, MaxSeconds: &seconds}, func(id string) (*ChatCompletionControlResponse, error) {
		controlCalled <- id
		return &ChatCompletionControlResponse{Success: true}, nil
	})
	w.setID("chatcmpl-time-only")
	w.start()
	select {
	case id := <-controlCalled:
		if id != "chatcmpl-time-only" {
			t.Fatalf("control called for %q", id)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("time-only cutoff did not trigger")
	}

	snapshot := w.snapshotCopy()
	if !snapshot.Triggered || snapshot.Reason != ReasoningCutoffReasonMaxSeconds || !snapshot.ControlSuccess {
		t.Fatalf("unexpected time-only cutoff snapshot: %+v", snapshot)
	}
}

func TestReasoningWatchdogDelaysUntilCompletionID(t *testing.T) {
	tokens, calls := 1, 0
	w := newReasoningWatchdog(&ReasoningCutoffConfig{Enabled: true, MaxTokens: &tokens}, func(id string) (*ChatCompletionControlResponse, error) {
		calls++
		if id != "chatcmpl-test" {
			return nil, errors.New("wrong ID")
		}
		return &ChatCompletionControlResponse{Success: true}, nil
	})
	w.start()
	w.update(1, nil, nil)
	if calls != 0 {
		t.Fatalf("control called before ID: %d", calls)
	}
	w.setID("chatcmpl-test")
	if calls != 1 {
		t.Fatalf("control calls=%d", calls)
	}
}

func TestReasoningWatchdogUsesLiveDecodedMetric(t *testing.T) {
	tokens, calls := 3, 0
	w := newReasoningWatchdog(&ReasoningCutoffConfig{Enabled: true, MaxTokens: &tokens}, func(string) (*ChatCompletionControlResponse, error) {
		calls++
		return &ChatCompletionControlResponse{Success: true}, nil
	})
	w.setID("chatcmpl-test")
	w.start()
	w.updateMetrics(41, 141, 200)
	w.updateMetrics(44, 144, 200)
	s := w.snapshotCopy()
	if calls != 1 || !s.Triggered || s.Reason != ReasoningCutoffReasonMaxTokens || s.ReasoningTokens != 3 {
		t.Fatalf("calls=%d snapshot=%+v", calls, s)
	}
}

func TestReasoningWatchdogNaturalEndSuppressesCutoff(t *testing.T) {
	tokens, calls := 1, 0
	w := newReasoningWatchdog(&ReasoningCutoffConfig{Enabled: true, MaxTokens: &tokens}, func(string) (*ChatCompletionControlResponse, error) {
		calls++
		return &ChatCompletionControlResponse{Success: true}, nil
	})
	w.setID("chatcmpl-test")
	w.start()
	w.end()
	w.update(1, nil, nil)
	if calls != 0 || w.snapshotCopy().Triggered {
		t.Fatalf("natural end did not suppress cutoff: calls=%d snapshot=%+v", calls, w.snapshotCopy())
	}
}

func TestEndReasoningRequest(t *testing.T) {
	var body ChatCompletionControlRequest
	client := NewClient(nil, "http://llama", WithHTTPClient(&http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Path != "/v1/chat/completions/control" {
			t.Fatalf("unexpected control request: %s %s", req.Method, req.URL.Path)
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(`{"success":true,"message":"accepted"}`)), Header: make(http.Header)}, nil
	})}))
	result, err := client.EndReasoning(context.Background(), "MODEL_ALIAS", "chatcmpl-test")
	if err != nil || result == nil || !result.Success {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if body.ID != "chatcmpl-test" || body.Action != "reasoning_end" || body.Model != "MODEL_ALIAS" {
		t.Fatalf("unexpected control body: %+v", body)
	}
}

func TestReasoningCutoffConfigKeyNames(t *testing.T) {
	for reason, want := range map[ReasoningCutoffReason]string{
		ReasoningCutoffReasonMaxTokens:  "maxTokens",
		ReasoningCutoffReasonMaxSeconds: "maxSeconds",
		ReasoningCutoffReasonMaxContext: "maxContextPercent",
	} {
		if got := reasoningCutoffConfigKey(reason); got != want {
			t.Errorf("%s mapped to %q, want %q", reason, got, want)
		}
	}
}
