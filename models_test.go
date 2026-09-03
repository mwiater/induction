package induction

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"strings"
	"testing"
)

// testStringer is a tiny helper type for exercising fmt.Stringer branches.
type testStringer string

// String returns the wrapped string value.
func (s testStringer) String() string { return string(s) }

func TestChatRequestMarshalsExtendedInferenceParameters(t *testing.T) {
	stream := false
	temperature := 0.0
	topK := 0
	ignoreEOS := false
	req := ChatRequest{
		Model:       "model-x",
		Prompt:      []int{1, 2, 3},
		Stream:      &stream,
		Temperature: &temperature,
		TopK:        &topK,
		IgnoreEOS:   &ignoreEOS,
		Stop:        []string{"END", "STOP"},
		ResponseFormat: &ResponseFormat{
			Type:       "json_schema",
			JSONSchema: map[string]any{"type": "object"},
		},
		Tools: []Tool{{
			Type: "function",
			Function: ToolFunction{
				Name:       "lookup",
				Parameters: map[string]any{"type": "object"},
			},
		}},
		ImageData: []ImageData{{Data: "aW1hZ2U=", ID: 7}},
	}

	payload, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("failed to decode request: %v", err)
	}
	for _, key := range []string{"prompt", "stream", "temperature", "top_k", "ignore_eos", "stop", "response_format", "tools", "image_data"} {
		if _, ok := got[key]; !ok {
			t.Errorf("expected %q in payload: %s", key, payload)
		}
	}
	if got["stream"] != false || got["temperature"] != float64(0) || got["top_k"] != float64(0) || got["ignore_eos"] != false {
		t.Fatalf("explicit zero values were not preserved: %s", payload)
	}
}

// TestModelHelpers covers the helper functions used by model table rendering.
func TestModelHelpers(t *testing.T) {
	if got := stringValue(nil); got != "" {
		t.Fatalf("expected empty string for nil, got %q", got)
	}
	if got := stringValue(testStringer("stringer")); got != "stringer" {
		t.Fatalf("expected stringer value, got %q", got)
	}
	if got := stringValue(json.Number("12")); got != "12" {
		t.Fatalf("expected json number value, got %q", got)
	}
	if got := stringValue(9); got != "9" {
		t.Fatalf("expected default conversion, got %q", got)
	}

	if got := stringSlice(nil); got != nil {
		t.Fatalf("expected nil slice, got %#v", got)
	}
	if got := stringSlice([]string{"a", "b"}); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("expected []string passthrough, got %#v", got)
	}
	if got := stringSlice([]interface{}{"a", 2}); len(got) != 2 || got[1] != "2" {
		t.Fatalf("expected []interface{} conversion, got %#v", got)
	}
	if got := stringSlice(3); len(got) != 1 || got[0] != "3" {
		t.Fatalf("expected scalar conversion, got %#v", got)
	}

	if got := joinStringSlice([]string{"a", "", "b"}); got != "a,b" {
		t.Fatalf("expected filtered join, got %q", got)
	}

	if got, ok := asMap(map[string]interface{}{"x": 1}); !ok || got["x"] != 1 {
		t.Fatalf("expected map conversion, got %#v %v", got, ok)
	}
	if _, ok := asMap(nil); ok {
		t.Fatal("expected nil conversion to fail")
	}

	if got := architectureValue(map[string]interface{}{"architecture": map[string]interface{}{"input_modalities": []interface{}{"text"}}}); got == nil {
		t.Fatal("expected architecture map")
	}
	if got := modalityStrings(nil, "input_modalities"); got != nil {
		t.Fatalf("expected nil modality strings, got %#v", got)
	}
	if got := modalityStrings(map[string]interface{}{}, "input_modalities"); got != nil {
		t.Fatalf("expected missing modality strings to be nil, got %#v", got)
	}

	nested := map[string]interface{}{
		"parameters": map[string]interface{}{"temperature": 0.3},
		"args":       map[string]interface{}{"top_p": 0.75},
	}
	if got := lookupModelValue(nested, "temperature"); got != "0.3" {
		t.Fatalf("expected nested parameter lookup, got %q", got)
	}
	if got := lookupModelValue(nested, "top-p", "top_p"); got != "0.75" {
		t.Fatalf("expected args lookup, got %q", got)
	}
	if got := lookupModelValue(nested, "missing"); got != "" {
		t.Fatalf("expected missing lookup to be empty, got %q", got)
	}

	row := modelRowFromMap(map[string]interface{}{
		"id":             "model-x",
		"loaded":         true,
		"temperature":    0.1,
		"repeat-last-n":  64,
		"repeat-penalty": 1.1,
		"top-k":          40,
		"top-p":          0.95,
		"architecture": map[string]interface{}{
			"input_modalities":  []interface{}{"text", "image"},
			"output_modalities": []string{"text"},
		},
	})
	if row.Name != "model-x" || !row.Loaded || row.Temperature != "0.1" || row.TopP != "0.95" {
		t.Fatalf("unexpected rendered row: %#v", row)
	}
	if row.InputModalities != "text,image" || row.OutputModalities != "text" {
		t.Fatalf("unexpected modality rendering: %#v", row)
	}
}

// TestDecodeModelRows covers both supported response shapes and the error case.
func TestDecodeModelRows(t *testing.T) {
	envelope, err := decodeModelRows(strings.NewReader(`{"data":[{"id":"envelope-model","loaded":true}]}`))
	if err != nil {
		t.Fatalf("decodeModelRows envelope failed: %v", err)
	}
	if len(envelope) != 1 || envelope[0].Name != "envelope-model" {
		t.Fatalf("unexpected envelope rows: %#v", envelope)
	}

	raw, err := decodeModelRows(strings.NewReader(`[{"id":"raw-model","args":{"top_p":0.5}}]`))
	if err != nil {
		t.Fatalf("decodeModelRows raw array failed: %v", err)
	}
	if len(raw) != 1 || raw[0].TopP != "0.5" {
		t.Fatalf("unexpected raw rows: %#v", raw)
	}

	if _, err := decodeModelRows(strings.NewReader(`not json`)); err == nil {
		t.Fatal("expected invalid JSON to fail")
	}
}

// TestClientHTTP_Fallback verifies the default HTTP client is available when unset.
func TestClientHTTP_Fallback(t *testing.T) {
	if got := (&Client{}).clientHTTP(); got == nil {
		t.Fatal("expected fallback HTTP client")
	}
}

// TestRenderModelTable_RendersHeaderAndLoadedMarker verifies the tabular output helper.
func TestRenderModelTable_RendersHeaderAndLoadedMarker(t *testing.T) {
	var buf bytes.Buffer
	renderModelTable(&buf, []modelRow{{Name: "loaded-model", Loaded: true}})
	output := buf.String()
	headerLine := strings.SplitN(output, "\n", 2)[0]
	expected := []string{"MODEL", "STATUS", "CTX", "BATCH", "UBATCH", "PARALLEL", "CACHE-K", "CACHE-V", "FLASH-ATTN", "TEMPERATURE", "TOP-K", "TOP-P", "REPEAT-LAST-N", "REPEAT-PENALTY", "INPUT MODALITIES", "OUTPUT MODALITIES"}
	pos := 0
	for _, want := range expected {
		idx := strings.Index(headerLine[pos:], want)
		if idx < 0 {
			t.Fatalf("expected header to contain %q in order, got %q", want, headerLine)
		}
		pos += idx + len(want)
	}

	if !strings.Contains(output, "LOADED") {
		t.Fatalf("expected loaded marker in output, got %q", output)
	}
}

func TestLogTablePrefixesEveryRow(t *testing.T) {
	var output bytes.Buffer
	logger := log.New(&output, "app: ", 0)
	client := NewClient(nil, "", WithLogger(logger))
	client.logTable("HEADER\nrow-one\nrow-two\n")

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected each table row to be logged separately, got %q", output.String())
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "app: ") {
			t.Fatalf("expected every row to carry the logger prefix: %q", line)
		}
	}
}

func TestModelRowFromMap_PopulatesRuntimeArgsAndDefaults(t *testing.T) {
	row := modelRowFromMap(map[string]interface{}{
		"id": "runtime-model",
		"status": map[string]interface{}{
			"value": "unloaded",
			"args": []interface{}{
				"llama-server", "--ctx-size", "131072", "--batch-size", "2048",
				"--ubatch-size", "512", "--parallel", "1", "--cache-type-k", "q8_0",
				"--cache-type-v", "q8_0", "--flash-attn", "on",
			},
		},
		"architecture": map[string]interface{}{
			"input_modalities": []interface{}{"text"}, "output_modalities": []interface{}{"text"},
		},
	})

	if row.ContextSize != "131072" || row.BatchSize != "2048" || row.UBatchSize != "512" || row.Parallel != "1" {
		t.Fatalf("runtime settings were not populated: %#v", row)
	}
	if row.CacheTypeK != "q8_0" || row.CacheTypeV != "q8_0" || row.FlashAttention != "on" {
		t.Fatalf("cache settings were not populated: %#v", row)
	}
	if row.Temperature != "DEFAULT" || row.TopK != "DEFAULT" || row.TopP != "DEFAULT" {
		t.Fatalf("omitted sampling settings should be explicit: %#v", row)
	}

	var buf bytes.Buffer
	renderModelTable(&buf, []modelRow{row})
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[1], "UNLOADED") {
		t.Fatalf("expected aligned header and explicit status, got %q", buf.String())
	}
}

// TestDecodeModelRows_ReadError covers the reader failure branch.
func TestDecodeModelRows_ReadError(t *testing.T) {
	_, err := decodeModelRows(errReader{})
	if err == nil {
		t.Fatal("expected read error")
	}
}

// errReader is an io.Reader that always fails.
type errReader struct{}

// Read always returns a failure so error handling paths can be tested.
func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// TestParsedModelArgs covers flat argv-style argument parsing.
func TestParsedModelArgs(t *testing.T) {
	args := parsedModelArgs([]interface{}{
		"--temperature", "0.0",
		"--repeat-last-n", "256",
		"--repeat-penalty", "1.02",
		"--top-k", "1",
		"--top-p", "1.0",
		"--mlock",
	})

	for key, want := range map[string]string{
		"temperature":    "0.0",
		"repeat-last-n":  "256",
		"repeat-penalty": "1.02",
		"top-k":          "1",
		"top-p":          "1.0",
		"mlock":          "true",
	} {
		if got := args[key]; got != want {
			t.Fatalf("expected %s=%s, got %q", key, want, got)
		}
	}
}

// TestModelRowFromMap_ArgsArray covers the table row extraction from flat args arrays.
func TestModelRowFromMap_ArgsArray(t *testing.T) {
	row := modelRowFromMap(map[string]interface{}{
		"id": "Granite-4.1-30B-Q4_K_M",
		"args": []interface{}{
			"--temperature", "0.0",
			"--repeat-last-n", "256",
			"--repeat-penalty", "1.02",
			"--top-k", "1",
			"--top-p", "1.0",
			"--mlock",
		},
		"architecture": map[string]interface{}{
			"input_modalities":  []interface{}{"text", "image"},
			"output_modalities": []interface{}{"text"},
		},
	})

	if row.Temperature != "0.0" || row.RepeatLastN != "256" || row.RepeatPenalty != "1.02" {
		t.Fatalf("unexpected parsed row values: %#v", row)
	}
	if row.TopK != "1" || row.TopP != "1.0" {
		t.Fatalf("unexpected parsed numeric columns: %#v", row)
	}
	if row.InputModalities != "text,image" || row.OutputModalities != "text" {
		t.Fatalf("unexpected parsed modalities: %#v", row)
	}
}

// TestModelRowFromMap_StatusArgsArray covers nested status.args extraction.
func TestModelRowFromMap_StatusArgsArray(t *testing.T) {
	row := modelRowFromMap(map[string]interface{}{
		"id": "Agents-A1-35B-Q4_K_M",
		"status": map[string]interface{}{
			"value": "unloaded",
			"args": []interface{}{
				"C:\\ai\\bin\\llama.cpp\\llama-server.exe",
				"--repeat-last-n", "256",
				"--repeat-penalty", "1.0",
				"--temperature", "0.85",
				"--top-k", "20",
				"--top-p", "0.95",
			},
		},
		"architecture": map[string]interface{}{
			"input_modalities":  []interface{}{"text", "image"},
			"output_modalities": []interface{}{"text"},
		},
	})

	if row.Temperature != "0.85" || row.RepeatLastN != "256" || row.RepeatPenalty != "1.0" {
		t.Fatalf("unexpected parsed row values: %#v", row)
	}
	if row.TopK != "20" || row.TopP != "0.95" {
		t.Fatalf("unexpected parsed numeric columns: %#v", row)
	}
	if row.InputModalities != "text,image" || row.OutputModalities != "text" {
		t.Fatalf("unexpected parsed modalities: %#v", row)
	}
}
