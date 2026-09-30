package induction

import (
	"encoding/json"
	"fmt"
	"strings"

	kg "github.com/mwiater/induction/internal/knowledgegraph"
)

func normalizeExtractionOutput(data []byte, item any) ([]byte, error) {
	var response struct {
		Entities  []kg.EntityObservation   `json:"entities"`
		Relations []kg.RelationObservation `json:"relations"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		// Some model/template combinations wrap extraction results in an
		// array. Accept wrapper objects and ignore unrelated scalar items (for
		// example a stray page number) rather than aborting the whole fan-out.
		var items []json.RawMessage
		if arrayErr := json.Unmarshal(data, &items); arrayErr != nil {
			// A generation may be cut off at maxTokens. An incomplete model
			// response cannot contribute trustworthy observations, so treat this
			// chunk as empty rather than aborting the complete corpus run.
			return []byte(`{"entities":[],"relations":[]}`), nil
		}
		for _, item := range items {
			var wrapper struct {
				Entities  []kg.EntityObservation   `json:"entities"`
				Relations []kg.RelationObservation `json:"relations"`
			}
			if json.Unmarshal(item, &wrapper) == nil {
				response.Entities = append(response.Entities, wrapper.Entities...)
				response.Relations = append(response.Relations, wrapper.Relations...)
			}
		}
	}
	obj, ok := item.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("fan-out chunk is not an object")
	}
	prov, ok := obj["provenance"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("chunk provenance is missing")
	}
	provenanceData, _ := json.Marshal(prov)
	var provenance kg.ChunkProvenance
	if err := json.Unmarshal(provenanceData, &provenance); err != nil {
		return nil, err
	}
	chunkText, _ := obj["text"].(string)
	validEntities := response.Entities[:0]
	for i := range response.Entities {
		canonical, ok := kg.CanonicalEvidencePrefix(response.Entities[i].Evidence, chunkText)
		if !ok {
			// Model-generated observations are advisory. Never allow an
			// unsupported observation into the graph, but keep other valid
			// observations from the same chunk.
			continue
		}
		response.Entities[i].Evidence = canonical
		response.Entities[i].Provenance = provenance
		response.Entities[i].ID = kg.ObservationID("entobs_", provenance.DocumentID, provenance.ChunkID, kg.Normalize(response.Entities[i].Name), kg.Normalize(response.Entities[i].Evidence))
		validEntities = append(validEntities, response.Entities[i])
	}
	response.Entities = validEntities
	validRelations := response.Relations[:0]
	entityNames := make(map[string]bool, len(response.Entities))
	for _, entity := range response.Entities {
		entityNames[kg.Normalize(entity.Name)] = true
	}
	for i := range response.Relations {
		canonical, ok := kg.CanonicalEvidencePrefix(response.Relations[i].Evidence, chunkText)
		if !ok {
			continue
		}
		if !entityNames[kg.Normalize(response.Relations[i].Source)] {
			if _, ok := kg.CanonicalEvidence(response.Relations[i].Source, chunkText); !ok {
				continue
			}
		}
		if !entityNames[kg.Normalize(response.Relations[i].Target)] {
			if _, ok := kg.CanonicalEvidence(response.Relations[i].Target, chunkText); !ok {
				continue
			}
		}
		response.Relations[i].Evidence = canonical
		response.Relations[i].Provenance = provenance
		response.Relations[i].ID = kg.ObservationID("relobs_", provenance.DocumentID, provenance.ChunkID, kg.Normalize(response.Relations[i].Source), kg.Normalize(response.Relations[i].Predicate), kg.Normalize(response.Relations[i].Target), kg.Normalize(response.Relations[i].Evidence))
		validRelations = append(validRelations, response.Relations[i])
	}
	response.Relations = validRelations
	// Every accepted relation endpoint must be represented by an entity
	// observation from this chunk. Models often emit sound relations while
	// omitting one endpoint from entities; recover endpoint strings that are
	// themselves present in the source chunk.
	for _, relation := range response.Relations {
		for _, endpoint := range []string{relation.Source, relation.Target} {
			key := kg.Normalize(endpoint)
			if key == "" || entityNames[key] {
				continue
			}
			evidence, ok := kg.CanonicalEvidence(endpoint, chunkText)
			if !ok {
				continue
			}
			response.Entities = append(response.Entities, kg.EntityObservation{
				ID:                   kg.ObservationID("entobs_", provenance.DocumentID, provenance.ChunkID, key, kg.Normalize(evidence)),
				Name:                 evidence,
				TypeHint:             "Unclassified",
				ContextualDefinition: "Relation endpoint explicitly supported by source evidence.",
				Evidence:             evidence,
				Provenance:           provenance,
			})
			entityNames[key] = true
		}
	}
	return json.Marshal(response)
}

func resolvePipelineForEach(expr string, outputs map[string]json.RawMessage, inputs map[string]any) ([]any, error) {
	value, err := resolvePipelineReferenceFor(strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(expr, "{{"), "}}")), outputs, inputs, nil, "item")
	if err != nil {
		return nil, err
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("forEach source must resolve to an array")
	}
	return items, nil
}

// pipelineJSONBytes preserves JSON supplied as a literal YAML string. Pipeline
// templates resolve references into native values, but a literal such as
// predicateResolutions: "[]" remains a Go string and must not be marshaled as
// the JSON string "[]" before decoding into its target type.
func pipelineJSONBytes(value any) []byte {
	if raw, ok := value.(string); ok {
		trimmed := strings.TrimSpace(raw)
		if json.Valid([]byte(trimmed)) {
			return []byte(trimmed)
		}
	}
	data, _ := json.Marshal(value)
	return data
}

func executePipelineTransform(step PipelineStep, outputs map[string]json.RawMessage) ([]byte, error) {
	input := map[string]any{}
	for key, raw := range step.Input {
		if expr, ok := raw.(string); ok && strings.HasPrefix(strings.TrimSpace(expr), "{{") && strings.HasSuffix(strings.TrimSpace(expr), "}}") {
			value, err := resolvePipelineReference(strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(expr), "{{"), "}}")), outputs, nil)
			if err != nil {
				return nil, err
			}
			input[key] = value
		} else {
			input[key] = raw
		}
	}
	decodeObservations := func() ([]kg.EntityObservation, []kg.RelationObservation, error) {
		value := input["observations"]
		data, err := json.Marshal(value)
		if err != nil {
			return nil, nil, err
		}
		var direct []kg.EntityObservation
		if err = json.Unmarshal(data, &direct); err == nil {
			// A fan-out result is an array of wrapper objects. JSON will
			// otherwise silently decode those wrappers as zero-valued direct
			// observations because unknown fields are ignored.
			allDirect := len(direct) == 0
			for _, observation := range direct {
				if observation.Name == "" || observation.ID == "" {
					allDirect = false
					break
				}
			}
			if allDirect {
				return direct, nil, nil
			}
		}
		eo, ro, err := flattenObservationItems(data, nil, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("observations must be an array: %w", err)
		}
		return eo, ro, nil
	}
	var result any
	var err error
	switch step.Transform {
	case "knowledgeGraph.entityCandidates":
		eo, _, err := decodeObservations()
		if err != nil {
			return nil, err
		}
		result, err = kg.GenerateEntityCandidates(eo)
		if err != nil {
			return nil, err
		}
	case "knowledgeGraph.canonicalizeEntities":
		var eo []kg.EntityObservation
		data, _ := json.Marshal(input["observations"])
		var ignored []kg.RelationObservation
		var err error
		eo, _, err = flattenObservationItems(data, eo, ignored)
		if err != nil {
			return nil, err
		}
		var candidates kg.EntityCandidates
		data, _ = json.Marshal(input["candidates"])
		if err := json.Unmarshal(data, &candidates); err != nil {
			return nil, err
		}
		var resolutions []kg.EntityResolution
		data, _ = json.Marshal(input["resolutions"])
		resolutions, err = decodeEntityResolutions(data)
		if err != nil {
			return nil, err
		}
		result, err = kg.CanonicalizeEntities(eo, candidates, resolutions)
		if err != nil {
			return nil, err
		}
	case "knowledgeGraph.predicateCandidates":
		var ro []kg.RelationObservation
		data, _ := json.Marshal(input["observations"])
		if err := json.Unmarshal(data, &ro); err != nil || (len(ro) > 0 && ro[0].ID == "") {
			var ignored []kg.EntityObservation
			var flattenErr error
			_, ro, flattenErr = flattenObservationItems(data, ignored, nil)
			if flattenErr != nil {
				return nil, flattenErr
			}
		}
		result = kg.GeneratePredicateCandidates(ro)
	case "knowledgeGraph.buildGraph":
		var eo []kg.EntityObservation
		var ro []kg.RelationObservation
		data, _ := json.Marshal(input["observations"])
		eo, ro, err = flattenObservationItems(data, eo, ro)
		if err != nil {
			return nil, err
		}
		var entities kg.CanonicalEntities
		data, _ = json.Marshal(input["entities"])
		if err := json.Unmarshal(data, &entities); err != nil {
			return nil, err
		}
		var pc kg.PredicateCandidates
		data, _ = json.Marshal(input["predicateCandidates"])
		if err := json.Unmarshal(data, &pc); err != nil {
			return nil, err
		}
		var pr []kg.PredicateResolution
		data = pipelineJSONBytes(input["predicateResolutions"])
		if err := json.Unmarshal(data, &pr); err != nil {
			return nil, err
		}
		result, err = kg.BuildGraph(eo, ro, entities, pc, pr)
		if err != nil {
			return nil, err
		}
	case "knowledgeGraph.finalize":
		var graph kg.Graph
		data, _ := json.Marshal(input["graph"])
		if err := json.Unmarshal(data, &graph); err != nil {
			return nil, err
		}
		var profile map[string]any
		data, _ = json.Marshal(input["profile"])
		_ = json.Unmarshal(data, &profile)
		data, _ = json.Marshal(input["semantics"])
		semantics, semanticsStatus, semanticsIssues := validatePipelineSemantics(data, graph.Entities)
		var entityObs []kg.EntityObservation
		var relationObs []kg.RelationObservation
		data, _ = json.Marshal(input["observations"])
		entityObs, relationObs, _ = flattenObservationItems(data, nil, nil)
		nodes := make([]any, 0, len(graph.Entities))
		for _, entity := range graph.Entities {
			nodes = append(nodes, map[string]any{"id": entity.ID, "canonical_name": entity.CanonicalName, "aliases": entity.Aliases, "type": entity.Type, "definition": entity.Definition, "observation_refs": entity.ObservationRefs})
		}
		edges := make([]any, 0, len(graph.Edges))
		for _, edge := range graph.Edges {
			edges = append(edges, edge)
		}
		result = map[string]any{"metadata": map[string]any{"schema_version": "1.0", "pipeline": "emergent-knowledge-graph-pipeline", "domain": profile["domain"], "domain_description": profile["domain_description"], "entity_observation_count": len(entityObs), "relation_observation_count": len(relationObs), "node_count": len(nodes), "edge_count": len(edges), "unresolved_relation_count": len(graph.UnresolvedRelations), "semantics_status": semanticsStatus, "semantics_issues": semanticsIssues, "weight_semantics": "support_density"}, "nodes": nodes, "edges": edges, "facets": semantics.Facets, "structural_hierarchy": semantics.StructuralHierarchy, "traversal_filters": semantics.TraversalFilters, "observations": map[string]any{"entities": entityObs, "relations": relationObs}, "unresolved_relations": graph.UnresolvedRelations}
	default:
		return nil, fmt.Errorf("unknown transform %q", step.Transform)
	}
	return json.Marshal(result)
}

// decodeEntityResolutions accepts the canonical {groups:[...]} form as well
// as the compact array-of-groups form commonly emitted by local models.
func validatePipelineSemantics(data []byte, entities []kg.CanonicalEntity) (kg.Semantics, string, []string) {
	semantics := kg.Semantics{
		Facets:              []kg.Facet{},
		StructuralHierarchy: []kg.StructuralRole{},
		TraversalFilters:    []kg.TraversalFilter{},
	}
	issues := []string{}
	nodes := make(map[string]bool, len(entities))
	for _, entity := range entities {
		nodes[entity.ID] = true
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil || object == nil {
		var facets []kg.Facet
		if arrayErr := json.Unmarshal(data, &facets); arrayErr == nil && facets != nil {
			facetsJSON, _ := json.Marshal(facets)
			object = map[string]json.RawMessage{
				"facets":               facetsJSON,
				"structural_hierarchy": json.RawMessage(`[]`),
				"traversal_filters":    json.RawMessage(`[]`),
			}
			issues = append(issues, "model returned a facets array; structural_hierarchy and traversal_filters are missing")
		} else {
			return semantics, "invalid", []string{"semantics output is not a JSON object or facets array"}
		}
	}
	// Accept the common single-facet response as partial output, but make the
	// schema mismatch visible instead of silently dropping the model result.
	if _, hasFacets := object["facets"]; !hasFacets {
		var facet kg.Facet
		if json.Unmarshal(data, &facet) == nil && strings.TrimSpace(facet.Name) != "" && facet.NodeIDs != nil {
			object["facets"], _ = json.Marshal([]kg.Facet{facet})
			object["structural_hierarchy"] = json.RawMessage(`[]`)
			object["traversal_filters"] = json.RawMessage(`[]`)
			issues = append(issues, "model returned a single facet; structural_hierarchy and traversal_filters are missing")
		}
	}
	if raw, ok := object["facets"]; !ok {
		issues = append(issues, "missing facets array")
	} else {
		var facets []kg.Facet
		if err := json.Unmarshal(raw, &facets); err != nil || facets == nil {
			issues = append(issues, "facets must be an array")
		} else {
			for i, facet := range facets {
				if strings.TrimSpace(facet.Name) == "" || strings.TrimSpace(facet.Description) == "" {
					issues = append(issues, fmt.Sprintf("facet %d is missing a name or description", i))
					continue
				}
				validIDs := make([]string, 0, len(facet.NodeIDs))
				seen := map[string]bool{}
				for _, id := range facet.NodeIDs {
					if !nodes[id] {
						issues = append(issues, fmt.Sprintf("facet %q references unknown node %q", facet.Name, id))
						continue
					}
					if !seen[id] {
						seen[id] = true
						validIDs = append(validIDs, id)
					}
				}
				if len(validIDs) == 0 {
					issues = append(issues, fmt.Sprintf("facet %q has no valid node IDs", facet.Name))
					continue
				}
				facet.NodeIDs = validIDs
				semantics.Facets = append(semantics.Facets, facet)
			}
		}
	}
	if raw, ok := object["structural_hierarchy"]; !ok {
		issues = append(issues, "missing structural_hierarchy array")
	} else {
		var roles []kg.StructuralRole
		if err := json.Unmarshal(raw, &roles); err != nil || roles == nil {
			issues = append(issues, "structural_hierarchy must be an array")
		} else {
			for _, role := range roles {
				if !nodes[role.NodeID] {
					issues = append(issues, fmt.Sprintf("structural role references unknown node %q", role.NodeID))
					continue
				}
				switch strings.ToLower(strings.TrimSpace(role.Role)) {
				case "hub", "intermediate", "leaf":
					role.Role = strings.ToLower(strings.TrimSpace(role.Role))
					semantics.StructuralHierarchy = append(semantics.StructuralHierarchy, role)
				default:
					issues = append(issues, fmt.Sprintf("invalid structural role %q for node %q", role.Role, role.NodeID))
				}
			}
		}
	}
	if raw, ok := object["traversal_filters"]; !ok {
		issues = append(issues, "missing traversal_filters array")
	} else {
		var filters []kg.TraversalFilter
		if err := json.Unmarshal(raw, &filters); err != nil || filters == nil {
			issues = append(issues, "traversal_filters must be an array")
		} else {
			for _, filter := range filters {
				if strings.TrimSpace(filter.Name) == "" || strings.TrimSpace(filter.Description) == "" {
					issues = append(issues, "traversal filter is missing a name or description")
					continue
				}
				semantics.TraversalFilters = append(semantics.TraversalFilters, filter)
			}
		}
	}
	if len(issues) == 0 {
		return semantics, "valid", issues
	}
	if len(semantics.Facets)+len(semantics.StructuralHierarchy)+len(semantics.TraversalFilters) == 0 {
		return semantics, "invalid", issues
	}
	return semantics, "partial", issues
}

func decodeEntityResolutions(data []byte) ([]kg.EntityResolution, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	result := make([]kg.EntityResolution, 0, len(items))
	for _, item := range items {
		var resolution kg.EntityResolution
		if json.Unmarshal(item, &resolution) == nil && resolution.Groups != nil {
			complete := true
			for _, group := range resolution.Groups {
				if len(group.ObservationIDs) == 0 {
					complete = false
					break
				}
			}
			if complete {
				result = append(result, resolution)
				continue
			}
			var wrapped struct {
				Groups []struct {
					ObservationIDs []string `json:"observation_ids"`
					Members        []string `json:"members"`
					CanonicalName  string   `json:"canonical_name"`
					Type           string   `json:"type"`
					Definition     string   `json:"definition"`
				} `json:"groups"`
			}
			if json.Unmarshal(item, &wrapped) == nil {
				converted := kg.EntityResolution{}
				for _, group := range wrapped.Groups {
					ids := group.ObservationIDs
					if len(ids) == 0 {
						ids = group.Members
					}
					converted.Groups = append(converted.Groups, kg.ResolutionGroup{ObservationIDs: ids, CanonicalName: group.CanonicalName, Type: group.Type, Definition: group.Definition})
				}
				if len(converted.Groups) > 0 {
					result = append(result, converted)
					continue
				}
			}
		}
		var groups []struct {
			ObservationIDs []string `json:"observation_ids"`
			Members        []string `json:"members"`
			CanonicalName  string   `json:"canonical_name"`
			Type           string   `json:"type"`
			Definition     string   `json:"definition"`
		}
		if err := json.Unmarshal(item, &groups); err != nil {
			var group struct {
				ObservationIDs []string `json:"observation_ids"`
				Members        []string `json:"members"`
				CanonicalName  string   `json:"canonical_name"`
				Type           string   `json:"type"`
				Definition     string   `json:"definition"`
			}
			if err := json.Unmarshal(item, &group); err != nil || group.CanonicalName == "" {
				continue
			}
			groups = append(groups, struct {
				ObservationIDs []string `json:"observation_ids"`
				Members        []string `json:"members"`
				CanonicalName  string   `json:"canonical_name"`
				Type           string   `json:"type"`
				Definition     string   `json:"definition"`
			}{ObservationIDs: group.ObservationIDs, Members: group.Members, CanonicalName: group.CanonicalName, Type: group.Type, Definition: group.Definition})
		}
		converted := kg.EntityResolution{}
		for _, group := range groups {
			ids := group.ObservationIDs
			if len(ids) == 0 {
				ids = group.Members
			}
			converted.Groups = append(converted.Groups, kg.ResolutionGroup{ObservationIDs: ids, CanonicalName: group.CanonicalName, Type: group.Type, Definition: group.Definition})
		}
		if len(converted.Groups) > 0 {
			result = append(result, converted)
		}
	}
	return result, nil
}

func flattenObservationItems(data []byte, entities []kg.EntityObservation, relations []kg.RelationObservation) ([]kg.EntityObservation, []kg.RelationObservation, error) {
	var direct struct {
		Entities  []kg.EntityObservation   `json:"entities"`
		Relations []kg.RelationObservation `json:"relations"`
	}
	if json.Unmarshal(data, &direct) == nil && (direct.Entities != nil || direct.Relations != nil) {
		return append(entities, direct.Entities...), append(relations, direct.Relations...), nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, nil, err
	}
	for _, item := range items {
		var wrapper struct {
			Entities  []kg.EntityObservation   `json:"entities"`
			Relations []kg.RelationObservation `json:"relations"`
		}
		if err := json.Unmarshal(item, &wrapper); err == nil && (wrapper.Entities != nil || wrapper.Relations != nil) {
			entities = append(entities, wrapper.Entities...)
			relations = append(relations, wrapper.Relations...)
		}
	}
	return entities, relations, nil
}
