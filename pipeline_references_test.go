package induction

import (
	"encoding/json"
	"strings"
	"testing"

	kg "github.com/mwiater/induction/internal/knowledgegraph"
)

func TestResolvePipelineDocumentChunks(t *testing.T) {
	chunk := map[string]any{"text": "source"}
	direct := map[string]any{"documents": map[string]any{"chunks": []any{chunk}}}
	for name, inputs := range map[string]map[string]any{
		"direct":  direct,
		"wrapped": {"inputs": direct},
	} {
		t.Run(name, func(t *testing.T) {
			value, err := resolvePipelineReferenceFor("inputs.documents.chunks", nil, inputs, nil, "item")
			if err != nil {
				t.Fatal(err)
			}
			items, ok := value.([]any)
			if !ok || len(items) != 1 {
				t.Fatalf("unexpected chunks: %#v", value)
			}
			if got := items[0].(map[string]any)["text"]; got != "source" {
				t.Fatalf("text = %#v", got)
			}
		})
	}
}

func TestRenderPipelineChunkReference(t *testing.T) {
	inputs := map[string]any{"documents": map[string]any{"chunks": []any{}}}
	out, err := renderPipelineReferencesFor("chunk={{ chunk.text }}", nil, inputs, map[string]any{"text": "source"}, "chunk")
	if err != nil {
		t.Fatal(err)
	}
	if out != "chunk=source" {
		t.Fatalf("rendered %q", out)
	}
}

func TestSplitPipelineDocumentTextPreservesExactSubstrings(t *testing.T) {
	text := "alpha beta\n gamma delta\n epsilon"
	chunks := splitPipelineDocumentText(text, 10)
	for _, chunk := range chunks {
		if !strings.Contains(text, chunk) {
			t.Fatalf("chunk is not an exact source substring: %q", chunk)
		}
	}
	joined := strings.Join(chunks, "")
	if joined != text {
		t.Fatalf("chunks changed source text: %q", joined)
	}
}

func TestNormalizePipelineDocumentTextJoinsLayoutWrappedLines(t *testing.T) {
	got := normalizePipelineDocumentText("first line\nwrapped sentence\n\nnew paragraph\n")
	want := "first line wrapped sentence\n\nnew paragraph"
	if got != want {
		t.Fatalf("normalized text = %q, want %q", got, want)
	}
}

func TestRecoverStructuredPipelineContentFromReasoning(t *testing.T) {
	step := PipelineStep{Output: &StepOutputConfig{Type: "json"}}
	snapshot := &ModelSnapshot{Interaction: []Interaction{{Content: "", ReasoningContent: `{"entities":[],"relations":[]}`}}}
	if got := recoverStructuredPipelineContent(step, "", snapshot); got != `{"entities":[],"relations":[]}` {
		t.Fatalf("recovered content = %q", got)
	}
	if got := recoverStructuredPipelineContent(PipelineStep{}, "", snapshot); got != "" {
		t.Fatalf("non-structured content = %q", got)
	}
}

func TestExtractStructuredJSON(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "prose", in: "Here is the result:\n{\"domain\":\"physics\"}\n", want: `{"domain":"physics"}`},
		{name: "fence", in: "```json\n{\"domain\":\"physics\"}\n```", want: `{"domain":"physics"}`},
		{name: "braces in string", in: `Answer: {"text":"a {b} value"}`, want: `{"text":"a {b} value"}`},
		{name: "invalid", in: "not JSON", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractStructuredJSON(tt.in); got != tt.want {
				t.Fatalf("extractStructuredJSON() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCanonicalEvidenceRestoresSourceText(t *testing.T) {
	chunk := "A posthuman civilization may run ancestor‐simulations."
	got, ok := kg.CanonicalEvidence("A POSTHUMAN civilization may run ancestor-simulations.", chunk)
	if !ok || got != chunk {
		t.Fatalf("canonical evidence = %q, %v; want source chunk", got, ok)
	}
	if _, ok := kg.CanonicalEvidence("A civilization may run simulations.", chunk); ok {
		t.Fatal("paraphrased evidence should not be accepted")
	}
}

func TestNormalizeExtractionOutputDropsTruncatedJSON(t *testing.T) {
	item := map[string]any{"text": "source", "provenance": map[string]any{
		"document_id": "doc", "chunk_id": "chunk", "chunk_index": 0,
	}}
	got, err := normalizeExtractionOutput([]byte(`{"entities":[`), item)
	if err != nil || string(got) != `{"entities":[],"relations":[]}` {
		t.Fatalf("normalized truncated extraction = %q, error=%v", got, err)
	}
}

func TestNormalizeExtractionOutputAddsEvidenceBackedRelationEndpoints(t *testing.T) {
	item := map[string]any{"text": "The red fox crosses the river.", "provenance": map[string]any{
		"document_id": "doc", "chunk_id": "chunk", "chunk_index": 0,
	}}
	input := `{"entities":[{"name":"red fox","type_hint":"Animal","contextual_definition":"fox","evidence":"The red fox crosses the river."}],"relations":[{"source":"red fox","predicate":"crosses","target":"river","evidence":"The red fox crosses the river."}]}`
	got, err := normalizeExtractionOutput([]byte(input), item)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Entities  []kg.EntityObservation   `json:"entities"`
		Relations []kg.RelationObservation `json:"relations"`
	}
	if err := json.Unmarshal(got, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Relations) != 1 {
		t.Fatalf("relations = %d, want 1", len(result.Relations))
	}
	if len(result.Entities) != 2 {
		t.Fatalf("entities = %d, want existing entity plus recovered endpoint", len(result.Entities))
	}
	if result.Entities[1].Name != "river" || result.Entities[1].TypeHint != "Unclassified" {
		t.Fatalf("recovered endpoint = %+v", result.Entities[1])
	}
}

func TestNormalizeExtractionOutputDropsRelationWithUngroundedEndpoint(t *testing.T) {
	item := map[string]any{"text": "The red fox crosses the river.", "provenance": map[string]any{
		"document_id": "doc", "chunk_id": "chunk", "chunk_index": 0,
	}}
	input := `{"entities":[],"relations":[{"source":"red fox","predicate":"crosses","target":"ocean","evidence":"The red fox crosses the river."}]}`
	got, err := normalizeExtractionOutput([]byte(input), item)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Entities  []kg.EntityObservation   `json:"entities"`
		Relations []kg.RelationObservation `json:"relations"`
	}
	if err := json.Unmarshal(got, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Relations) != 0 || len(result.Entities) != 0 {
		t.Fatalf("unsupported relation was retained: %+v", result)
	}
}

func TestNormalizeExtractionOutputAcceptsEndpointsGroundedInChunk(t *testing.T) {
	item := map[string]any{"text": "The red fox crosses the river.", "provenance": map[string]any{
		"document_id": "doc", "chunk_id": "chunk", "chunk_index": 0,
	}}
	input := `{"entities":[],"relations":[{"source":"fox","predicate":"crosses","target":"river","evidence":"The red fox crosses the river."}]}`
	got, err := normalizeExtractionOutput([]byte(input), item)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Entities  []kg.EntityObservation   `json:"entities"`
		Relations []kg.RelationObservation `json:"relations"`
	}
	if err := json.Unmarshal(got, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Relations) != 1 || len(result.Entities) != 2 {
		t.Fatalf("chunk-grounded relation was not retained and recovered: %+v", result)
	}
}

func TestValidatePipelineSemanticsPreservesPartialFacetAndReportsMissingFields(t *testing.T) {
	nodes := []kg.CanonicalEntity{{ID: "node-1"}}
	got, status, issues := validatePipelineSemantics([]byte(`{"name":"topic","description":"one topic","node_ids":["node-1"]}`), nodes)
	if status != "partial" || len(issues) == 0 {
		t.Fatalf("status=%q issues=%v, want partial output with diagnostics", status, issues)
	}
	if len(got.Facets) != 1 || got.Facets[0].NodeIDs[0] != "node-1" {
		t.Fatalf("single-facet response was not preserved: %+v", got)
	}
	if got.StructuralHierarchy == nil || got.TraversalFilters == nil {
		t.Fatalf("missing semantics arrays should be normalized to empty arrays: %+v", got)
	}
}

func TestValidatePipelineSemanticsAcceptsFacetArrayAsPartialOutput(t *testing.T) {
	input := `[{"name":"topic","description":"topic facet","node_ids":["node-1"]}]`
	got, status, issues := validatePipelineSemantics([]byte(input), []kg.CanonicalEntity{{ID: "node-1"}})
	if status != "partial" || len(issues) == 0 {
		t.Fatalf("status=%q issues=%v, want partial output with diagnostics", status, issues)
	}
	if len(got.Facets) != 1 || got.Facets[0].Name != "topic" {
		t.Fatalf("facet array was not preserved: %+v", got.Facets)
	}
	if got.StructuralHierarchy == nil || got.TraversalFilters == nil {
		t.Fatalf("missing semantics arrays should be normalized: %+v", got)
	}
}

func TestValidatePipelineSemanticsFiltersInvalidReferencesAndRoles(t *testing.T) {
	input := `{"facets":[{"name":"topic","description":"topic facet","node_ids":["node-1","missing"]}],"structural_hierarchy":[{"node_id":"missing","role":"hub"},{"node_id":"node-1","role":"root"}],"traversal_filters":[{"name":"focus","description":"filter"}]}`
	got, status, issues := validatePipelineSemantics([]byte(input), []kg.CanonicalEntity{{ID: "node-1"}})
	if status != "partial" || len(issues) < 3 {
		t.Fatalf("status=%q issues=%v, want partial output and validation diagnostics", status, issues)
	}
	if len(got.Facets) != 1 || len(got.Facets[0].NodeIDs) != 1 || got.Facets[0].NodeIDs[0] != "node-1" {
		t.Fatalf("invalid facet references were not filtered: %+v", got.Facets)
	}
	if len(got.StructuralHierarchy) != 0 {
		t.Fatalf("invalid structural roles were retained: %+v", got.StructuralHierarchy)
	}
	if len(got.TraversalFilters) != 1 {
		t.Fatalf("valid traversal filter was dropped: %+v", got.TraversalFilters)
	}
}
