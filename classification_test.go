package induction

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
		{"empty label", PipelineStep{Name: "classify", Model: "model", UserPrompt: "x", Classification: &ClassificationConfig{Candidates: map[string]string{"A": "", "B": "two"}}}, "value"},
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

func TestPipelineDecisionAndConditionValidation(t *testing.T) {
	maxTokensTwo := 2
	p := Pipeline{Name: "routing", Steps: []PipelineStep{
		{Name: "gate", Model: "model", UserPrompt: "choose", Decision: &DecisionConfig{Candidates: map[string]string{"A": "yes", "B": "no"}}},
		{Name: "yes", Model: "model", UserPrompt: "continue", When: &WhenCondition{Decision: "gate", Equals: "yes", MinConfidence: floatPtr(0.8)}},
	}}
	if err := p.Validate(); err != nil {
		t.Fatalf("valid decision pipeline rejected: %v", err)
	}
	if p.Steps[0].Decision.TopLogprobs != 20 {
		t.Fatalf("decision topLogprobs default = %d, want 20", p.Steps[0].Decision.TopLogprobs)
	}

	tests := []struct {
		name  string
		steps []PipelineStep
		want  string
	}{
		{"both keys", []PipelineStep{{Name: "gate", Model: "m", UserPrompt: "x", Decision: &DecisionConfig{Candidates: map[string]string{"A": "a", "B": "b"}}, Classification: &ClassificationConfig{Candidates: map[string]string{"A": "a", "B": "b"}}}}, "cannot both"},
		{"future reference", []PipelineStep{{Name: "use", Model: "m", UserPrompt: "x", When: &WhenCondition{Decision: "gate", Equals: "yes"}}, {Name: "gate", Model: "m", UserPrompt: "x", Decision: &DecisionConfig{Candidates: map[string]string{"A": "yes", "B": "no"}}}}, "earlier"},
		{"unknown value", []PipelineStep{{Name: "gate", Model: "m", UserPrompt: "x", Decision: &DecisionConfig{Candidates: map[string]string{"A": "yes", "B": "no"}}}, {Name: "use", Model: "m", UserPrompt: "x", When: &WhenCondition{Decision: "gate", Equals: "maybe"}}}, "not a value"},
		{"unknown decision", []PipelineStep{{Name: "use", Model: "m", UserPrompt: "x", When: &WhenCondition{Decision: "missing", Equals: "yes"}}}, "unknown decision"},
		{"generative reference", []PipelineStep{{Name: "draft", Model: "m", UserPrompt: "x"}, {Name: "use", Model: "m", UserPrompt: "x", When: &WhenCondition{Decision: "draft", Equals: "yes"}}}, "not a decision step"},
		{"threshold out of range", []PipelineStep{{Name: "gate", Model: "m", UserPrompt: "x", Decision: &DecisionConfig{Candidates: map[string]string{"A": "yes", "B": "no"}}}, {Name: "use", Model: "m", UserPrompt: "x", When: &WhenCondition{Decision: "gate", Equals: "yes", MinConfidence: floatPtr(1.1)}}}, "minConfidence must be in [0,1]"},
		{"empty condition value", []PipelineStep{{Name: "gate", Model: "m", UserPrompt: "x", Decision: &DecisionConfig{Candidates: map[string]string{"A": "yes", "B": "no"}}}, {Name: "use", Model: "m", UserPrompt: "x", When: &WhenCondition{Decision: "gate", Equals: " "}}}, "when.decision and when.equals are required"},
		{"decision maxTokens conflict", []PipelineStep{{Name: "gate", Model: "m", UserPrompt: "x", Decision: &DecisionConfig{Candidates: map[string]string{"A": "yes", "B": "no"}}, Parameters: &PipelineParameters{MaxTokens: &maxTokensTwo}}}, "requires maxTokens=1"},
		{"decision structured output conflict", []PipelineStep{{Name: "gate", Model: "m", UserPrompt: "x", Decision: &DecisionConfig{Candidates: map[string]string{"A": "yes", "B": "no"}}, JSONSchema: map[string]any{"type": "object"}}}, "cannot be combined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline := Pipeline{Name: "test", Steps: tt.steps}
			if err := pipeline.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestEvaluatePipelineCondition(t *testing.T) {
	data, _ := json.Marshal(DecisionResult{Type: "decision", SelectedValue: "security", Confidence: 0.9, Margin: 0.7})
	outputs := map[string]json.RawMessage{"gate": data}
	if ok, err := evaluatePipelineCondition(&WhenCondition{Decision: "gate", Equals: "security", MinConfidence: floatPtr(0.8), MinMargin: floatPtr(0.7)}, outputs); err != nil || !ok {
		t.Fatalf("expected condition to pass; got %t, %v", ok, err)
	}
	if ok, err := evaluatePipelineCondition(&WhenCondition{Decision: "gate", Equals: "security", MinMargin: floatPtr(0.8)}, outputs); err != nil || ok {
		t.Fatalf("expected margin condition to fail; got %t, %v", ok, err)
	}
}

func TestEvaluatePipelineConditionRejectsUnavailableOrInvalidResults(t *testing.T) {
	for _, tc := range []struct {
		name    string
		outputs map[string]json.RawMessage
		want    string
	}{
		{"missing result", map[string]json.RawMessage{}, "with no result"},
		{"malformed result", map[string]json.RawMessage{"gate": json.RawMessage(`{`)}, "decode decision"},
		{"not a decision", map[string]json.RawMessage{"gate": json.RawMessage(`{"type":"skipped"}`)}, "did not produce a decision"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := evaluatePipelineCondition(&WhenCondition{Decision: "gate", Equals: "yes"}, tc.outputs)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("condition error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestBuildDecisionResultStableTieAndOrdering(t *testing.T) {
	cfg := &DecisionConfig{Candidates: map[string]string{"C": "third", "A": "first", "B": "second"}, TopLogprobs: 3}
	result, err := buildDecisionResult(cfg, map[string]float64{"A": -1, "B": -1, "C": -2})
	if err != nil {
		t.Fatal(err)
	}
	if result.SelectedCandidate != "A" || result.Margin != 0 {
		t.Fatalf("unexpected tie result: %#v", result)
	}
	if len(result.Candidates) != 3 || result.Candidates[0].Candidate != "A" || result.Candidates[1].Candidate != "B" || result.Candidates[2].Candidate != "C" {
		t.Fatalf("candidate order is unstable: %#v", result.Candidates)
	}
}

func TestBuildDecisionResultRejectsIncompleteOrInvalidScores(t *testing.T) {
	cfg := &DecisionConfig{Candidates: map[string]string{"A": "yes", "B": "no"}, TopLogprobs: 2}
	for _, tc := range []struct {
		name   string
		scores map[string]float64
		want   string
	}{
		{"missing configured candidate", map[string]float64{"A": -0.1}, `did not include candidate "B"`},
		{"NaN logprob", map[string]float64{"A": math.NaN(), "B": -1}, `candidate "A"`},
		{"infinite logprob", map[string]float64{"A": -1, "B": math.Inf(1)}, `candidate "B"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := buildDecisionResult(cfg, tc.scores); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("buildDecisionResult error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestValidateDecisionConfigRejectsInvalidConfigurations(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  *DecisionConfig
		want string
	}{
		{"nil", nil, "at least two"},
		{"one candidate", &DecisionConfig{Candidates: map[string]string{"A": "yes"}, TopLogprobs: 1}, "at least two"},
		{"empty key", &DecisionConfig{Candidates: map[string]string{"": "yes", "B": "no"}, TopLogprobs: 2}, "key cannot be empty"},
		{"empty value", &DecisionConfig{Candidates: map[string]string{"A": " ", "B": "no"}, TopLogprobs: 2}, `value for candidate "A"`},
		{"zero top logprobs", &DecisionConfig{Candidates: map[string]string{"A": "yes", "B": "no"}}, "must be positive"},
		{"too few top logprobs", &DecisionConfig{Candidates: map[string]string{"A": "yes", "B": "no"}, TopLogprobs: 1}, "at least the number"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateDecisionConfig(tc.cfg); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("validateDecisionConfig error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestTokenizeDecisionCandidateRejectsMalformedResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"zero tokens", http.StatusOK, `{"tokens":[]}`, "tokenizes to 0 tokens"},
		{"malformed token", http.StatusOK, `{"tokens":[{}]}`, "did not return a token piece"},
		{"invalid json", http.StatusOK, `not-json`, "decode tokenization response"},
		{"server error", http.StatusInternalServerError, `tokenizer unavailable`, "tokenization returned 500"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client := NewClient(context.Background(), server.URL)
			if _, err := client.tokenizeClassificationCandidate(context.Background(), "model", "A"); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("tokenization error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestDecisionRejectsMalformedCompletionResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"http error", http.StatusBadGateway, "unavailable", "returned 502"},
		{"invalid json", http.StatusOK, "not-json", "failed to decode decision response"},
		{"no choices", http.StatusOK, `{"choices":[]}`, "contained no choices"},
		{"no logprobs", http.StatusOK, `{"choices":[{}]}`, "did not contain logprobs"},
		{"empty token data", http.StatusOK, `{"choices":[{"logprobs":{"content":[]}}]}`, "no next-token probability data"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/tokenize" {
					var request struct {
						Content string `json:"content"`
					}
					_ = json.NewDecoder(r.Body).Decode(&request)
					_, _ = fmt.Fprintf(w, `{"tokens":[{"id":1,"piece":%q}]}`, request.Content)
					return
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client := NewClient(context.Background(), server.URL)
			_, err := client.doDecision(context.Background(), &ChatRequest{Model: "model", Decision: &DecisionConfig{Candidates: map[string]string{"A": "yes", "B": "no"}, TopLogprobs: 2}})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("decision error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestSkippedPipelineStepMakesNoInferenceRequest(t *testing.T) {
	decisionJSON, _ := json.Marshal(DecisionResult{Type: "decision", SelectedValue: "irrelevant", Confidence: 0.95, Margin: 0.9})
	m := &consoleModel{
		pipeline: &Pipeline{Steps: []PipelineStep{
			{Name: "gate", Decision: &DecisionConfig{Candidates: map[string]string{"A": "relevant", "B": "irrelevant"}}},
			{Name: "analyze", When: &WhenCondition{Decision: "gate", Equals: "relevant"}},
		}},
		pipelineOutputs: map[string]json.RawMessage{"gate": decisionJSON},
		pipelineRunning: true,
	}
	cmd := m.startPipelineStep(1)
	if cmd == nil {
		t.Fatal("expected pipeline progression command")
	}
	if len(m.messages) != 1 || !strings.Contains(m.messages[0].content, "Skipped step") {
		t.Fatalf("skip was not recorded: %#v", m.messages)
	}
	var status map[string]any
	if err := json.Unmarshal(m.pipelineOutputs["analyze"], &status); err != nil || status["type"] != "skipped" {
		t.Fatalf("skipped state not retained: %s, %v", m.pipelineOutputs["analyze"], err)
	}
	// The step exits through the skip path before touching the client or
	// scheduling a model load / chat completion.
}

func TestMatchingPipelineConditionSchedulesInference(t *testing.T) {
	decisionJSON, _ := json.Marshal(DecisionResult{Type: "decision", SelectedValue: "security", Confidence: 0.92, Margin: 0.84})
	client := NewClient(context.Background(), "http://unused.invalid")
	inferenceCalls := 0
	m := &consoleModel{
		ctx: context.Background(), client: client, timeout: time.Second,
		request: ChatRequest{Model: "model"},
		pipeline: &Pipeline{Steps: []PipelineStep{
			{Name: "gate", Decision: &DecisionConfig{Candidates: map[string]string{"A": "security", "B": "other"}}},
			{Name: "security-review", Model: "model", UserPrompt: "review", When: &WhenCondition{Decision: "gate", Equals: "security", MinConfidence: floatPtr(0.9)}},
		}},
		pipelineOutputs: map[string]json.RawMessage{"gate": decisionJSON},
		inferSnapshotTurn: func(_ context.Context, request *ChatRequest) (*InferenceResponse, *ModelSnapshot, error) {
			inferenceCalls++
			if len(request.Messages) == 0 || request.Messages[len(request.Messages)-1].Content != "review" {
				t.Errorf("rendered request did not contain the step prompt: %#v", request.Messages)
			}
			return &InferenceResponse{Choices: []InferenceChoice{{Message: &InferenceResponseMessage{Content: "review result"}}}}, nil, nil
		},
	}
	cmd := m.startPipelineStep(1)
	if cmd == nil {
		t.Fatal("matching condition did not schedule a turn")
	}
	if _, ok := cmd().(consoleTurnResult); !ok {
		t.Fatal("scheduled command did not return an inference result")
	}
	if inferenceCalls != 1 {
		t.Fatalf("inference calls = %d, want 1", inferenceCalls)
	}
	if len(m.messages) != 1 || m.messages[0].role != "user" {
		t.Fatalf("matching step was not submitted: %#v", m.messages)
	}
}

func TestDecisionCannotUseStreamingAPI(t *testing.T) {
	client := NewClient(context.Background(), "http://unused.invalid")
	_, err := client.GenerateStreamingSnapshot(context.Background(), &ChatRequest{Model: "model", Decision: &DecisionConfig{Candidates: map[string]string{"A": "yes", "B": "no"}}}, func(InferenceStreamChunk) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "decision inference is non-streaming") {
		t.Fatalf("streaming decision error = %v", err)
	}
}

func floatPtr(value float64) *float64 { return &value }

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
	var result DecisionResult
	if err := json.Unmarshal([]byte(interaction.Content), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Type != "decision" || result.SelectedCandidate != "A" || result.SelectedValue != "one" || result.Confidence <= result.Candidates[1].Probability {
		t.Fatalf("unexpected decision result from legacy classification: %#v", result)
	}
	probabilitySum := 0.0
	for _, candidate := range result.Candidates {
		probabilitySum += candidate.Probability
	}
	if probabilitySum < 0.999999 || probabilitySum > 1.000001 {
		t.Fatalf("probabilities sum to %f, want approximately 1", probabilitySum)
	}
	if interaction.Response == "" {
		t.Fatal("raw response was not retained")
	}
	canonical, err := client.doDecision(context.Background(), &ChatRequest{Model: "model", Messages: []Message{{Role: "user", Content: "Choose"}}, Decision: &DecisionConfig{Candidates: map[string]string{"A": "one", "B": "two"}, TopLogprobs: 20}})
	if err != nil {
		t.Fatalf("canonical decision execution failed: %v", err)
	}
	var canonicalResult DecisionResult
	if err := json.Unmarshal([]byte(canonical.Content), &canonicalResult); err != nil || canonicalResult.SelectedValue != "one" {
		t.Fatalf("unexpected canonical decision result: %#v (error %v)", canonicalResult, err)
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
	if err == nil || !strings.Contains(err.Error(), `did not include candidate "B"`) {
		t.Fatalf("missing candidate error = %v", err)
	}
}
