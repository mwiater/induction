package knowledgegraph

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIdentifiersEvidenceAndSerialization(t *testing.T) {
	documentSame := DocumentID([]byte("same"))
	documentDifferent := DocumentID([]byte("different"))
	if documentSame == documentDifferent || documentSame != documentSame {
		t.Fatal("document IDs are not deterministic and content-derived")
	}
	chunkOne := ChunkID("doc", 1, "text")
	chunkTwo := ChunkID("doc", 2, "text")
	observationSame := ObservationID("obs_", "a", "b")
	observationOther := ObservationID("obs_", "a", "b")
	if chunkOne == chunkTwo || observationSame != observationOther {
		t.Fatal("identifier derivation is incorrect")
	}
	if got := Normalize("  Hello,   WORLD! "); got != "hello, world" {
		t.Fatalf("Normalize = %q", got)
	}
	chunk := "The “quick—brown” fox. It jumps over the lazy dog."
	if got, ok := CanonicalEvidence(" QUICK—BROWN ", chunk); !ok || got != "quick—brown" {
		t.Fatalf("normalized evidence = %q, %v", got, ok)
	}
	if VerifyEvidence("missing", chunk) || VerifyEvidence("", chunk) {
		t.Fatal("invalid evidence was accepted")
	}
	if got, ok := CanonicalEvidencePrefix("The “quick—brown” fox. It jumps over the lazy dog. It continues outside the chunk", chunk); !ok || got != chunk {
		t.Fatalf("evidence prefix = %q, %v", got, ok)
	}
	b, err := Serialize(map[string]string{"<key>": "&value"})
	if err != nil || !strings.Contains(string(b), "<key>") || strings.Contains(string(b), `\\u003c`) {
		t.Fatalf("Serialize = %q, %v", b, err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(b, &decoded); err != nil || decoded["<key>"] != "&value" {
		t.Fatalf("serialized JSON did not round-trip: %v", err)
	}
}

func TestCandidateGenerationAndResolutionValidation(t *testing.T) {
	p := ChunkProvenance{DocumentID: "doc", ChunkID: "chunk"}
	obs := []EntityObservation{
		{ID: "a", Name: "New York City", TypeHint: "place", Provenance: p},
		{ID: "b", Name: "NYC", TypeHint: "place", Provenance: p},
		{ID: "c", Name: "Unrelated", TypeHint: "thing", Provenance: p},
	}
	candidates, err := GenerateEntityCandidates(obs)
	if err != nil || len(candidates.Candidates) != 1 || candidates.Candidates[0].Signals[0] != "acronym-match" || len(candidates.Singletons) != 1 {
		t.Fatalf("candidates = %+v, %v", candidates, err)
	}
	if err := ValidateEntityResolution(candidates.Candidates[0], EntityResolution{Groups: []ResolutionGroup{{ObservationIDs: []string{"a", "b"}, CanonicalName: "New York City"}}}, map[string]EntityObservation{"a": obs[0], "b": obs[1]}); err != nil {
		t.Fatalf("valid entity resolution rejected: %v", err)
	}
	for _, resolution := range []EntityResolution{{}, {Groups: []ResolutionGroup{{ObservationIDs: []string{"a"}, CanonicalName: "missing"}}}, {Groups: []ResolutionGroup{{ObservationIDs: []string{"a", "a"}, CanonicalName: "New York City"}}}} {
		if err := ValidateEntityResolution(candidates.Candidates[0], resolution, map[string]EntityObservation{"a": obs[0], "b": obs[1]}); err == nil {
			t.Errorf("invalid entity resolution was accepted: %+v", resolution)
		}
	}
	relations := []RelationObservation{{ID: "r1", Predicate: "supports"}, {ID: "r2", Predicate: "Supports."}, {ID: "r3", Predicate: "uses"}}
	pc := GeneratePredicateCandidates(relations)
	if len(pc.Candidates) != 1 || len(pc.Singletons) != 1 || pc.Candidates[0].Signals[0] != "normalized-equality" {
		t.Fatalf("predicate candidates = %+v", pc)
	}
	if err := ValidatePredicateResolution(pc.Candidates[0], PredicateResolution{Groups: []PredicateGroup{{Predicates: []string{"supports", "Supports."}, CanonicalPredicate: "supports"}}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePredicateResolution(pc.Candidates[0], PredicateResolution{}); err == nil {
		t.Fatal("incomplete predicate resolution was accepted")
	}
}

func TestValidateGraphRejectsBrokenReferences(t *testing.T) {
	p := ChunkProvenance{DocumentID: "doc", ChunkID: "chunk"}
	node := CanonicalEntity{ID: "n", ObservationRefs: []string{"e"}}
	entities := map[string]EntityObservation{"e": {ID: "e", Provenance: p}}
	relations := map[string]RelationObservation{"r": {ID: "r", Provenance: p}}
	docs := map[string]bool{"doc": true}
	if err := ValidateGraph(Graph{Entities: []CanonicalEntity{node}}, entities, relations, docs); err != nil {
		t.Fatal(err)
	}
	bad := []Graph{
		{Entities: []CanonicalEntity{node, node}},
		{Entities: []CanonicalEntity{node}, Edges: []Edge{{ID: "x", Source: "missing", Target: "n", ObservationRefs: []string{"r"}}}},
		{Entities: []CanonicalEntity{node}, Edges: []Edge{{ID: "x", Source: "n", Target: "n"}}},
		{Entities: []CanonicalEntity{node}, Edges: []Edge{{ID: "x", Source: "n", Target: "n", ObservationRefs: []string{"missing"}}}},
		{Entities: []CanonicalEntity{node}, UnresolvedRelations: []UnresolvedRelation{{RelationObservationID: "missing"}}},
	}
	for _, graph := range bad {
		if err := ValidateGraph(graph, entities, relations, docs); err == nil {
			t.Errorf("invalid graph was accepted: %+v", graph)
		}
	}
}
