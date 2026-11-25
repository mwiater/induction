package knowledgegraph

import "testing"

func TestBuildGraphUsesFallbackPredicateResolution(t *testing.T) {
	provenance := ChunkProvenance{DocumentID: "doc", ChunkID: "chunk"}
	entityObs := []EntityObservation{
		{ID: "ent-a", Name: "A", Provenance: provenance},
		{ID: "ent-b", Name: "B", Provenance: provenance},
	}
	relationObs := []RelationObservation{{
		ID: "rel-1", Source: "A", Predicate: "supports", Target: "B", Provenance: provenance,
	}}
	entities := CanonicalEntities{
		Entities: []CanonicalEntity{
			{ID: "node-a", CanonicalName: "A", ObservationRefs: []string{"ent-a"}},
			{ID: "node-b", CanonicalName: "B", ObservationRefs: []string{"ent-b"}},
		},
		ObservationToEntity: map[string]string{"ent-a": "node-a", "ent-b": "node-b"},
	}
	predicates := PredicateCandidates{Candidates: []PredicateCandidate{{
		ID: "pred-1", Predicates: []string{"supports", "supports."}, RelationObservationRefs: []string{"rel-1"},
	}}}

	graph, err := BuildGraph(entityObs, relationObs, entities, predicates, nil)
	if err != nil {
		t.Fatalf("BuildGraph returned error without predicate resolutions: %v", err)
	}
	if len(graph.Edges) != 1 || graph.Edges[0].Predicate != "supports" {
		t.Fatalf("unexpected fallback graph edges: %+v", graph.Edges)
	}
}

func TestBuildGraphResolvesCanonicalNamesAndAliasesFromChunkMentions(t *testing.T) {
	provenance := ChunkProvenance{DocumentID: "doc", ChunkID: "chunk"}
	entityObs := []EntityObservation{
		{ID: "ent-a", Name: "Alpha system", Provenance: provenance},
		{ID: "ent-b", Name: "Beta system", Provenance: provenance},
	}
	relationObs := []RelationObservation{{
		ID: "rel-1", Source: "Alpha", Predicate: "connects", Target: "Beta", Provenance: provenance,
	}}
	entities := CanonicalEntities{
		Entities: []CanonicalEntity{
			{ID: "node-a", CanonicalName: "Alpha", Aliases: []string{"Alpha system"}, ObservationRefs: []string{"ent-a"}},
			{ID: "node-b", CanonicalName: "Beta", Aliases: []string{"Beta system"}, ObservationRefs: []string{"ent-b"}},
		},
		ObservationToEntity: map[string]string{"ent-a": "node-a", "ent-b": "node-b"},
	}
	graph, err := BuildGraph(entityObs, relationObs, entities, PredicateCandidates{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Edges) != 1 || graph.Edges[0].Source != "node-a" || graph.Edges[0].Target != "node-b" {
		t.Fatalf("canonical endpoint names did not resolve through chunk mentions: %+v", graph)
	}
}

func TestCanonicalizeEntitiesMergesDuplicateCanonicalNames(t *testing.T) {
	provenance := ChunkProvenance{DocumentID: "doc", ChunkID: "chunk"}
	obs := []EntityObservation{
		{ID: "ent-a", Name: "Simulation Scenario", TypeHint: "Philosophy", ContextualDefinition: "first", Provenance: provenance},
		{ID: "ent-b", Name: "simulation scenario", TypeHint: "Philosophy", ContextualDefinition: "a longer definition", Provenance: provenance},
	}
	entities, err := CanonicalizeEntities(obs, EntityCandidates{Singletons: []string{"ent-a", "ent-b"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entities.Entities) != 1 {
		t.Fatalf("duplicate canonical names were not merged: %+v", entities.Entities)
	}
	if got := len(entities.Entities[0].ObservationRefs); got != 2 {
		t.Fatalf("merged observation refs = %d, want 2", got)
	}
	if entities.ObservationToEntity["ent-a"] != entities.ObservationToEntity["ent-b"] {
		t.Fatalf("merged observations map to different entities: %#v", entities.ObservationToEntity)
	}
}
