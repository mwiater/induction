package induction

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPipelineClassificationValidation(t *testing.T) {
	base := Pipeline{Name: "classification", Steps: []PipelineStep{{Name: "classify", Model: "model", UserPrompt: "Classify", Classification: &ClassificationConfig{Candidates: map[string]string{"A": "one", "B": "two"}}}}}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid classification rejected: %v", err)
	}
	if got := base.Steps[0].Classification.TopLogprobs; got != 20 {
		t.Fatalf("default topLogprobs = %d, want 20", got)
	}

	tests := []struct {
		name string
		step PipelineStep
		want string
	}{
		{"too few candidates", PipelineStep{Name: "classify", Model: "model", UserPrompt: "x", Classification: &ClassificationConfig{Candidates: map[string]string{"A": "one"}}}, "at least two"},
		{"empty label", PipelineStep{Name: "classify", Model: "model", UserPrompt: "x", Classification: &ClassificationConfig{Candidates: map[string]string{"A": "", "B": "two"}}}, "label"},
		{"too few top logprobs", PipelineStep{Name: "classify", Model: "model", UserPrompt: "x", Classification: &ClassificationConfig{Candidates: map[string]string{"A": "one", "B": "two"}, TopLogprobs: 1}}, "topLogprobs"},
		{"structured output", PipelineStep{Name: "classify", Model: "model", UserPrompt: "x", ResponseFormat: &ResponseFormat{Type: "json_object"}, Classification: &ClassificationConfig{Candidates: map[string]string{"A": "one", "B": "two"}}}, "cannot be combined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline := Pipeline{Name: "classification", Steps: []PipelineStep{tt.step}}
			if err := pipeline.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestClassificationScoresCandidatesNotGeneratedText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/tokenize":
			var request struct {
				Content string `json:"content"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode tokenize request: %v", err)
				return
			}
			_, _ = w.Write([]byte(`{"tokens":[{"id":1,"piece":"` + request.Content + `"}]}`))
		case "/v1/chat/completions":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode completion request: %v", err)
				return
			}
			kwargs, _ := request["chat_template_kwargs"].(map[string]any)
			if request["max_tokens"] != float64(1) || request["stream"] != false || request["logprobs"] != true || request["top_logprobs"] != float64(20) || kwargs["enable_thinking"] != false {
				t.Errorf("classification controls missing: %#v", request)
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"B"},"logprobs":{"content":[{"token":"B","logprob":-2.0,"top_logprobs":[{"token":"A","logprob":-0.1}] }]}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(context.Background(), server.URL)
	interaction, err := client.doClassification(context.Background(), &ChatRequest{Model: "model", Messages: []Message{{Role: "user", Content: "Choose"}}, Classification: &ClassificationConfig{Candidates: map[string]string{"A": "one", "B": "two"}, TopLogprobs: 20}})
	if err != nil {
		t.Fatalf("doClassification failed: %v", err)
	}
	var result ClassificationResult
	if err := json.Unmarshal([]byte(interaction.Content), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Class != "A" || result.Label != "one" || result.Confidence <= result.Probabilities["B"] {
		t.Fatalf("unexpected classification result: %#v", result)
	}
	probabilitySum := 0.0
	for _, probability := range result.Probabilities {
		probabilitySum += probability
	}
	if probabilitySum < 0.999999 || probabilitySum > 1.000001 {
		t.Fatalf("probabilities sum to %f, want approximately 1", probabilitySum)
	}
	if interaction.Response == "" {
		t.Fatal("raw response was not retained")
	}
}

func TestClassificationRejectsMultiTokenCandidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tokens":[{"id":1,"piece":"Y"},{"id":2,"piece":"ES"}]}`))
	}))
	defer server.Close()
	client := NewClient(context.Background(), server.URL)
	_, err := client.tokenizeClassificationCandidate(context.Background(), "model", "YES")
	if err == nil || !strings.Contains(err.Error(), "tokenizes to 2 tokens") {
		t.Fatalf("multi-token error = %v", err)
	}
}

func TestClassificationRejectsMissingCandidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/tokenize" {
			var request struct {
				Content string `json:"content"`
			}
			_ = json.NewDecoder(r.Body).Decode(&request)
			_, _ = w.Write([]byte(`{"tokens":[{"id":1,"piece":"` + request.Content + `"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"logprobs":{"content":[{"top_logprobs":[{"token":"A","logprob":-0.1}]}]}}]}`))
	}))
	defer server.Close()
	client := NewClient(context.Background(), server.URL)
	_, err := client.doClassification(context.Background(), &ChatRequest{Model: "model", Messages: []Message{{Role: "user", Content: "Choose"}}, Classification: &ClassificationConfig{Candidates: map[string]string{"A": "one", "B": "two"}, TopLogprobs: 2}})
	if err == nil || !strings.Contains(err.Error(), `candidate "B" was not present`) {
		t.Fatalf("missing candidate error = %v", err)
	}
}
