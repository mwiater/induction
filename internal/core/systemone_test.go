package induction

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestSystemOneDecisionUsesNativeEndpointAndAdaptsChoice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"JEV5K","output_modalities":["decisions"]}]}`))
			return
		}
		if r.URL.Path != "/v1/systemone" {
			t.Fatalf("path = %s, want /v1/systemone", r.URL.Path)
		}
		var request systemOneRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Model != "JEV5K" {
			t.Fatalf("model = %q, want JEV5K", request.Model)
		}
		question, ok := request.Questions["decision"]
		if !ok || question.Type != "choice" || question.Instructions != "Which route?" {
			t.Fatalf("unexpected question: %#v", request.Questions)
		}
		if request.State != "report text" {
			t.Fatalf("state = %#v, want report text", request.State)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"JEV5K","answers":{"decision":{"type":"choice","choice":"security","probabilities":{"security":0.8,"other":0.2},"confidence":0.6}}}`))
	}))
	defer server.Close()

	client := NewClient(context.Background(), server.URL)
	interaction, err := client.doDecision(context.Background(), &ChatRequest{
		Model:    "JEV5K",
		Messages: []Message{{Role: "user", Content: "ignored by explicit state"}},
		Decision: &DecisionConfig{
			Endpoint:     "systemone",
			Type:         "choice",
			State:        "report text",
			Instructions: "Which route?",
			Candidates:   map[string]string{"security": "Security", "other": "Other"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var result DecisionResult
	if err := json.Unmarshal([]byte(interaction.Content), &result); err != nil {
		t.Fatal(err)
	}
	if result.SelectedValue != "security" || result.Confidence != 0.8 || math.Abs(result.Margin-0.6) > 1e-9 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Candidates[0].Logprob != 0 || result.Candidates[1].Logprob != 0 {
		t.Fatalf("native decision should not expose legacy logprobs: %#v", result.Candidates)
	}
}

func TestDecisionRoutingPipelinesUseSystemOne(t *testing.T) {
	for _, name := range []string{"pipeline.decision-routing-01.yaml", "pipeline.decision-routing-02.yaml"} {
		pipeline, err := LoadPipeline(filepath.Join("../../pipelines", name))
		if err != nil {
			t.Fatalf("LoadPipeline(%s): %v", name, err)
		}
		decisionCount := 0
		for _, step := range pipeline.Steps {
			if step.Decision == nil {
				continue
			}
			decisionCount++
			if step.Decision.Endpoint != "systemone" {
				t.Fatalf("%s step %q is not a System One decision: %#v", name, step.Name, step.Decision)
			}
			if step.Decision.State == nil || step.Decision.Instructions == "" {
				t.Fatalf("%s step %q has incomplete native decision config: %#v", name, step.Name, step.Decision)
			}
			switch step.Decision.Type {
			case "choice":
				if len(step.Decision.Candidates) < 2 && step.Decision.CandidatesFrom == "" {
					t.Fatalf("%s step %q has incomplete choice config: %#v", name, step.Name, step.Decision)
				}
			case "noul":
				criteria, ok := step.Decision.Criteria.(map[string]interface{})
				if !ok || criteria["true"] == "" || criteria["false"] == "" {
					t.Fatalf("%s step %q has incomplete noul config: %#v", name, step.Name, step.Decision)
				}
			case "score":
				criteria, ok := step.Decision.Criteria.([]interface{})
				if !ok || len(criteria) < 2 || len(criteria) > 10 {
					t.Fatalf("%s step %q has incomplete score config: %#v", name, step.Name, step.Decision)
				}
			default:
				t.Fatalf("%s step %q has unsupported System One type %q", name, step.Name, step.Decision.Type)
			}
		}
		if decisionCount == 0 {
			t.Fatalf("%s contains no decision steps", name)
		}
	}
}
