package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	induction "github.com/mwiater/induction"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestApplicationTools(t *testing.T) {
	req := withLocalTools(&induction.ChatRequest{Model: "model"})
	if len(req.Tools) != 13 || req.ToolChoice != "auto" {
		t.Fatalf("local tools not attached: %#v", req)
	}

	calls := []induction.InferenceToolCall{{Function: induction.InferenceFunctionCall{Name: "other"}}}
	chained := chainLocalTools(calls)
	if len(chained) != 2 || chained[1].Function.Name != "get_current_datetime" {
		t.Fatalf("expected automatic time tool: %#v", chained)
	}
	if len(chainLocalTools([]induction.InferenceToolCall{{Function: induction.InferenceFunctionCall{Name: "get_current_datetime"}}})) != 1 {
		t.Fatal("existing time tool should not be duplicated")
	}

	if _, err := localToolResult(context.Background(), "unknown", "", ""); err == nil {
		t.Fatal("unknown local tool should fail")
	}
	if value, err := localToolResult(context.Background(), "get_current_datetime", "", ""); err != nil || !strings.Contains(value, "date_time") {
		t.Fatalf("date/time tool = %q, err=%v", value, err)
	}
}

func TestSystemToolsReturnStructuredJSON(t *testing.T) {
	for name, result := range map[string]func() (string, error){
		"disk":   func() (string, error) { return freeDiskSpace(context.Background()) },
		"memory": func() (string, error) { return availableMemory(context.Background()) },
	} {
		t.Run(name, func(t *testing.T) {
			value, err := result()
			if err != nil {
				t.Fatal(err)
			}
			var decoded any
			if err := json.Unmarshal([]byte(value), &decoded); err != nil {
				t.Fatalf("result is not JSON: %v", err)
			}
		})
	}
}

func TestFetchModelIDs(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, _ := json.Marshal(map[string]any{"data": []map[string]string{{"id": "model-a"}, {"id": ""}, {"id": "model-b"}}})
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
	})}
	value, err := fetchModelIDs(context.Background(), "http://llama", client)
	if err != nil {
		t.Fatal(err)
	}
	if value != `["model-a","model-b"]` {
		t.Fatalf("model IDs = %s", value)
	}
}

func TestCanonicalModelFamily(t *testing.T) {
	got := canonicalModelFamily("Qwen-3.6-35B-A3B-MTP-Coding-Q8_K_XL")
	if got != "qwen-3.6-35b" {
		t.Fatalf("family = %q", got)
	}
	if got := canonicalModelFamily("model-Q4_0-GGUF-00001-of-00002"); got != "model" {
		t.Fatalf("shard family = %q", got)
	}
}

func TestInspectAndProfileRuntimeModels(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body := `{"object":"list","data":[{"id":"Agents-A1-Apex-35B-A3B-MTP-I-Quality","context_length":262144,"architecture":{"input_modalities":["text","image"]},"status":{"args":["--model","/home/matt/ai/models/SC117/Agents-A1-MTP-APEX-GGUF/Agents-A1-MTP-APEX-I-Quality.gguf"]}},{"id":"Small-8B-Q4_0","context_length":32768,"architecture":{"input_modalities":["text"]},"status":{"preset":"[Small]\nmodel = /home/matt/models/author/repo/small.gguf\n"}}]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	value, err := inspectAndProfileRuntimeModels(context.Background(), "http://llama", client)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`"id":"Agents-A1-Apex-35B-A3B-MTP-I-Quality"`,
		`"family_slug":"agents-a1-apex-35b"`,
		`"local_path":"/home/matt/ai/models/SC117/Agents-A1-MTP-APEX-GGUF/Agents-A1-MTP-APEX-I-Quality.gguf"`,
		`"huggingface_url":"https://huggingface.co/SC117/Agents-A1-MTP-APEX-GGUF"`,
		`"huggingface_author":"SC117"`,
		`"huggingface_repo":"Agents-A1-MTP-APEX-GGUF"`,
		`"quantization_file":"Agents-A1-MTP-APEX-I-Quality.gguf"`,
		`"context_capacity":262144`,
		`"vision_supported":true`,
		`"inferred_parameters":"35B"`,
		`"architecture_flags":["MoE:A3B","MTP"]`,
		`"local_path":"/home/matt/models/author/repo/small.gguf"`,
	} {
		if !strings.Contains(value, expected) {
			t.Fatalf("missing %s in runtime profiles: %s", expected, value)
		}
	}
}
