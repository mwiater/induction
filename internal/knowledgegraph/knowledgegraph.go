// Package knowledgegraph contains the deterministic half of Induction's
// evidence-backed knowledge-graph pipeline. It deliberately has no model or
// UI dependency; Bubble Tea owns orchestration and calls these transforms.
package knowledgegraph

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"
)

// DocumentProvenance is an exported type in this package's public data model.
type DocumentProvenance struct {
	// DocumentID stores the value represented by this member.
	DocumentID string `json:"document_id"`
	// Path stores the value represented by this member.
	Path string `json:"path"`
	// SHA256 stores the value represented by this member.
	SHA256 string `json:"sha256"`
	// DisplayName stores the value represented by this member.
	DisplayName string `json:"display_name"`
}

// ChunkProvenance is an exported type in this package's public data model.
type ChunkProvenance struct {
	// ChunkID stores the value represented by this member.
	ChunkID string `json:"chunk_id"`
	// DocumentID stores the value represented by this member.
	DocumentID string `json:"document_id"`
	// ChunkIndex stores the value represented by this member.
	ChunkIndex int `json:"chunk_index"`
	// Page stores the value represented by this member.
	Page *int `json:"page,omitempty"`
	// StartOffset stores the value represented by this member.
	StartOffset *int `json:"start_offset,omitempty"`
	// EndOffset stores the value represented by this member.
	EndOffset *int `json:"end_offset,omitempty"`
}

// Chunk is an exported type in this package's public data model.
type Chunk struct {
	// Text stores the value represented by this member.
	Text string `json:"text"`
	// Provenance stores the value represented by this member.
	Provenance ChunkProvenance `json:"provenance"`
}

// EntityObservation is an exported type in this package's public data model.
type EntityObservation struct {
	// ID stores the value represented by this member.
	ID string `json:"id"`
	// Name stores the value represented by this member.
	Name string `json:"name"`
	// TypeHint stores the value represented by this member.
	TypeHint string `json:"type_hint"`
	// ContextualDefinition stores the value represented by this member.
	ContextualDefinition string `json:"contextual_definition"`
	// Evidence stores the value represented by this member.
	Evidence string `json:"evidence"`
	// Provenance stores the value represented by this member.
	Provenance ChunkProvenance `json:"provenance"`
}

// RelationObservation is an exported type in this package's public data model.
type RelationObservation struct {
	// ID stores the value represented by this member.
	ID string `json:"id"`
	// Source stores the value represented by this member.
	Source string `json:"source"`
	// Predicate stores the value represented by this member.
	Predicate string `json:"predicate"`
	// Target stores the value represented by this member.
	Target string `json:"target"`
	// Evidence stores the value represented by this member.
	Evidence string `json:"evidence"`
	// Provenance stores the value represented by this member.
	Provenance ChunkProvenance `json:"provenance"`
}

// EntityCandidate is an exported type in this package's public data model.
type EntityCandidate struct {
	// ID stores the value represented by this member.
	ID string `json:"id"`
	// Mentions stores the value represented by this member.
	Mentions []string `json:"mentions"`
	// Signals stores the value represented by this member.
	Signals []string `json:"signals"`
}

// EntityCandidates is an exported type in this package's public data model.
type EntityCandidates struct {
	// Candidates stores the value represented by this member.
	Candidates []EntityCandidate `json:"candidates"`
	// Singletons stores the value represented by this member.
	Singletons []string `json:"singletons"`
}

// ResolutionGroup is an exported type in this package's public data model.
type ResolutionGroup struct {
	// ObservationIDs stores the value represented by this member.
	ObservationIDs []string `json:"observation_ids"`
	// CanonicalName stores the value represented by this member.
	CanonicalName string `json:"canonical_name"`
	// Type stores the value represented by this member.
	Type string `json:"type"`
	// Definition stores the value represented by this member.
	Definition string `json:"definition"`
}

// EntityResolution is an exported type in this package's public data model.
type EntityResolution struct {
	// Groups stores the value represented by this member.
	Groups []ResolutionGroup `json:"groups"`
	// Reasoning stores the value represented by this member.
	Reasoning string `json:"reasoning,omitempty"`
}

// CanonicalEntity is an exported type in this package's public data model.
type CanonicalEntity struct {
	// ID stores the value represented by this member.
	ID string `json:"id"`
	// CanonicalName stores the value represented by this member.
	CanonicalName string `json:"canonical_name"`
	// Aliases stores the value represented by this member.
	Aliases []string `json:"aliases"`
	// Type stores the value represented by this member.
	Type string `json:"type"`
	// Definition stores the value represented by this member.
	Definition string `json:"definition"`
	// ObservationRefs stores the value represented by this member.
	ObservationRefs []string `json:"observation_refs"`
}

// CanonicalEntities is an exported type in this package's public data model.
type CanonicalEntities struct {
	// Entities stores the value represented by this member.
	Entities []CanonicalEntity `json:"entities"`
	// ObservationToEntity stores the value represented by this member.
	ObservationToEntity map[string]string `json:"observation_to_entity"`
}

// PredicateCandidate is an exported type in this package's public data model.
type PredicateCandidate struct {
	// ID stores the value represented by this member.
	ID string `json:"id"`
	// Predicates stores the value represented by this member.
	Predicates []string `json:"predicates"`
	// RelationObservationRefs stores the value represented by this member.
	RelationObservationRefs []string `json:"relation_observation_refs"`
	// Signals stores the value represented by this member.
	Signals []string `json:"signals"`
}

// PredicateSingleton is an exported type in this package's public data model.
type PredicateSingleton struct {
	// Predicate stores the value represented by this member.
	Predicate string `json:"predicate"`
	// RelationObservationRefs stores the value represented by this member.
	RelationObservationRefs []string `json:"relation_observation_refs"`
}

// PredicateCandidates is an exported type in this package's public data model.
type PredicateCandidates struct {
	// Candidates stores the value represented by this member.
	Candidates []PredicateCandidate `json:"candidates"`
	// Singletons stores the value represented by this member.
	Singletons []PredicateSingleton `json:"singletons"`
}

// PredicateGroup is an exported type in this package's public data model.
type PredicateGroup struct {
	// Predicates stores the value represented by this member.
	Predicates []string `json:"predicates"`
	// CanonicalPredicate stores the value represented by this member.
	CanonicalPredicate string `json:"canonical_predicate"`
	// Definition stores the value represented by this member.
	Definition string `json:"definition"`
}

// PredicateResolution is an exported type in this package's public data model.
type PredicateResolution struct {
	// Groups stores the value represented by this member.
	Groups []PredicateGroup `json:"groups"`
	// Reasoning stores the value represented by this member.
	Reasoning string `json:"reasoning,omitempty"`
}

// Edge is an exported type in this package's public data model.
type Edge struct {
	// ID stores the value represented by this member.
	ID string `json:"id"`
	// Source stores the value represented by this member.
	Source string `json:"source"`
	// Target stores the value represented by this member.
	Target string `json:"target"`
	// Predicate stores the value represented by this member.
	Predicate string `json:"predicate"`
	// PredicateFamily stores the value represented by this member.
	PredicateFamily string `json:"predicate_family"`
	// Weight stores the value represented by this member.
	Weight float64 `json:"weight"`
	// ObservationRefs stores the value represented by this member.
	ObservationRefs []string `json:"observation_refs"`
}

// UnresolvedRelation is an exported type in this package's public data model.
type UnresolvedRelation struct {
	// RelationObservationID stores the value represented by this member.
	RelationObservationID string `json:"relation_observation_id"`
	// Reason stores the value represented by this member.
	Reason string `json:"reason"`
}

// Graph is an exported type in this package's public data model.
type Graph struct {
	// Entities stores the value represented by this member.
	Entities []CanonicalEntity `json:"entities"`
	// Edges stores the value represented by this member.
	Edges []Edge `json:"edges"`
	// UnresolvedRelations stores the value represented by this member.
	UnresolvedRelations []UnresolvedRelation `json:"unresolved_relations,omitempty"`
}

// Facet is an exported type in this package's public data model.
type Facet struct {
	// Name stores the value represented by this member.
	Name string `json:"name"`
	// Description stores the value represented by this member.
	Description string `json:"description"`
	// NodeIDs stores the value represented by this member.
	NodeIDs []string `json:"node_ids"`
}

// StructuralRole is an exported type in this package's public data model.
type StructuralRole struct {
	// NodeID stores the value represented by this member.
	NodeID string `json:"node_id"`
	// Role stores the value represented by this member.
	Role string `json:"role"`
}

// TraversalFilter is an exported type in this package's public data model.
type TraversalFilter struct {
	// Name stores the value represented by this member.
	Name string `json:"name"`
	// Description stores the value represented by this member.
	Description string `json:"description"`
}

// Semantics is an exported type in this package's public data model.
type Semantics struct {
	// Facets stores the value represented by this member.
	Facets []Facet `json:"facets"`
	// StructuralHierarchy stores the value represented by this member.
	StructuralHierarchy []StructuralRole `json:"structural_hierarchy"`
	// TraversalFilters stores the value represented by this member.
	TraversalFilters []TraversalFilter `json:"traversal_filters"`
}

// hashID performs the internal operation described by its signature.
func hashID(prefix string, parts ...string) string {
	h := sha256.New()
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte("\n"))
		}
		h.Write([]byte(p))
	}
	return prefix + hex.EncodeToString(h.Sum(nil))[:20]
}

// DocumentID is an exported operation provided by this package.
func DocumentID(data []byte) string {
	h := sha256.Sum256(data)
	return "doc_" + hex.EncodeToString(h[:])[:20]
}

// ChunkID is an exported operation provided by this package.
func ChunkID(documentID string, index int, text string) string {
	return hashID("chunk_", documentID, fmt.Sprint(index), text)
}

// ObservationID is an exported operation provided by this package.
func ObservationID(prefix string, parts ...string) string { return hashID(prefix, parts...) }

// Normalize is an exported operation provided by this package.
func Normalize(s string) string {
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	s = strings.ToLower(s)
	s = strings.TrimFunc(s, func(r rune) bool { return unicode.IsPunct(r) && r != '-' && r != '/' })
	return s
}

// VerifyEvidence is an exported operation provided by this package.
func VerifyEvidence(evidence, chunk string) bool {
	_, ok := CanonicalEvidence(evidence, chunk)
	return ok
}

// CanonicalEvidence returns the exact source substring represented by
// evidence. The first comparison is byte-exact; the fallback tolerates model
// changes to Unicode punctuation, case, and whitespace, then returns the
// original source text so persisted evidence remains authoritative.
func CanonicalEvidence(evidence, chunk string) (string, bool) {
	if evidence == "" {
		return "", false
	}
	chunk = strings.ReplaceAll(chunk, "\r\n", "\n")
	evidence = strings.ReplaceAll(evidence, "\r\n", "\n")
	if strings.Contains(chunk, evidence) {
		return evidence, true
	}

	chunkRunes := []rune(chunk)
	normalized := make([]rune, 0, len(chunkRunes))
	starts := make([]int, 0, len(chunkRunes))
	ends := make([]int, 0, len(chunkRunes))
	space := false
	for i, r := range chunkRunes {
		r = normalizeEvidenceRune(r)
		if r == ' ' {
			if space {
				continue
			}
			space = true
		} else {
			space = false
		}
		normalized = append(normalized, r)
		starts = append(starts, i)
		ends = append(ends, i+1)
	}
	want := normalizeEvidenceString(evidence)
	if want == "" {
		return "", false
	}
	for i := 0; i+len([]rune(want)) <= len(normalized); i++ {
		candidate := string(normalized[i : i+len([]rune(want))])
		if strings.EqualFold(candidate, want) {
			return string(chunkRunes[starts[i]:ends[i+len([]rune(want))-1]]), true
		}
	}
	return "", false
}

// CanonicalEvidencePrefix recovers a complete source-backed prefix from an
// overlong model quote. This is useful when a model continues quoting past a
// fan-out chunk boundary. The returned value is always canonical source text.
func CanonicalEvidencePrefix(evidence, chunk string) (string, bool) {
	if exact, ok := CanonicalEvidence(evidence, chunk); ok {
		return exact, true
	}
	runes := []rune(strings.TrimSpace(evidence))
	for end := len(runes) - 1; end >= 32; end-- {
		last := runes[end-1]
		if last != '.' && last != '!' && last != '?' && last != ';' {
			continue
		}
		if candidate, ok := CanonicalEvidence(string(runes[:end]), chunk); ok {
			return candidate, true
		}
	}
	return "", false
}

// normalizeEvidenceString performs the internal operation described by its signature.
func normalizeEvidenceString(value string) string {
	var b strings.Builder
	space := false
	for _, r := range value {
		r = normalizeEvidenceRune(r)
		if r == ' ' {
			if space {
				continue
			}
			space = true
		} else {
			space = false
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// normalizeEvidenceRune performs the internal operation described by its signature.
func normalizeEvidenceRune(r rune) rune {
	if unicode.IsSpace(r) {
		return ' '
	}
	switch r {
	case '‐', '‑', '‒', '–', '—', '―', '−':
		return '-'
	case '“', '”':
		return '"'
	case '‘', '’':
		return '\''
	}
	return unicode.ToLower(r)
}

// GenerateEntityCandidates is an exported operation provided by this package.
func GenerateEntityCandidates(obs []EntityObservation) (EntityCandidates, error) {
	parent := make([]int, len(obs))
	for i := range parent {
		parent[i] = i
	}
	signals := map[[2]int]map[string]bool{}
	find := func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b int) {
		a, b = find(a), find(b)
		if a != b {
			parent[b] = a
		}
	}
	index := map[string][]int{}
	for i, o := range obs {
		index[Normalize(o.Name)] = append(index[Normalize(o.Name)], i)
	}
	for _, ids := range index {
		for i := 1; i < len(ids); i++ {
			union(ids[0], ids[i])
			signals[[2]int{ids[0], ids[i]}] = map[string]bool{"normalized-name-match": true}
		}
	}
	for i := range obs {
		for j := i + 1; j < len(obs); j++ {
			a, b := Normalize(obs[i].Name), Normalize(obs[j].Name)
			sig := candidateSignal(a, b, obs[i].TypeHint, obs[j].TypeHint)
			if sig != "" {
				union(i, j)
				signals[[2]int{i, j}] = map[string]bool{sig: true}
			}
		}
	}
	clusters := map[int][]int{}
	for i := range obs {
		clusters[find(i)] = append(clusters[find(i)], i)
	}
	result := EntityCandidates{}
	roots := make([]int, 0, len(clusters))
	for root := range clusters {
		roots = append(roots, root)
	}
	sort.Ints(roots)
	for _, root := range roots {
		ids := clusters[root]
		sort.Ints(ids)
		if len(ids) == 1 {
			result.Singletons = append(result.Singletons, obs[ids[0]].ID)
			continue
		}
		mentions := make([]string, 0, len(ids))
		signalSet := map[string]bool{}
		for _, i := range ids {
			mentions = append(mentions, obs[i].ID)
			for j := range ids {
				if i < j {
					for s := range signals[[2]int{i, j}] {
						signalSet[s] = true
					}
				}
			}
		}
		sigs := sortedKeys(signalSet)
		result.Candidates = append(result.Candidates, EntityCandidate{ID: hashID("entcand_", strings.Join(mentions, "\n")), Mentions: mentions, Signals: sigs})
	}
	return result, nil
}

// candidateSignal performs the internal operation described by its signature.
func candidateSignal(a, b, ta, tb string) string {
	if a == b {
		return "normalized-name-match"
	}
	if acronym(a) == b || acronym(b) == a {
		return "acronym-match"
	}
	short, long := a, b
	if len(short) > len(long) {
		short, long = long, short
	}
	if len(short) > 2 && tokenContains(long, short) && (ta == "" || tb == "" || Normalize(ta) == Normalize(tb)) {
		return "short-form-containment"
	}
	return ""
}

// acronym performs the internal operation described by its signature.
func acronym(s string) string {
	words := strings.Fields(strings.NewReplacer("-", " ", "/", " ", "(", " ", ")", " ").Replace(s))
	if len(words) < 2 {
		return ""
	}
	var b strings.Builder
	for _, w := range words {
		if w != "" {
			b.WriteByte(w[0])
		}
	}
	return b.String()
}

// tokenContains performs the internal operation described by its signature.
func tokenContains(long, short string) bool {
	lw, sw := strings.Fields(long), strings.Fields(short)
	if len(sw) == 0 || len(sw) > len(lw) {
		return false
	}
	for i := 0; i+len(sw) <= len(lw); i++ {
		if strings.Join(lw[i:i+len(sw)], " ") == strings.Join(sw, " ") {
			return true
		}
	}
	return false
}

// ValidateEntityResolution is an exported operation provided by this package.
func ValidateEntityResolution(candidate EntityCandidate, r EntityResolution, observed map[string]EntityObservation) error {
	expected := map[string]bool{}
	for _, id := range candidate.Mentions {
		expected[id] = true
	}
	seen := map[string]bool{}
	for _, g := range r.Groups {
		if len(g.ObservationIDs) == 0 {
			return fmt.Errorf("empty entity resolution group")
		}
		if _, ok := observed[g.CanonicalName]; !ok {
			found := false
			for _, id := range g.ObservationIDs {
				if o, ok := observed[id]; ok && (o.Name == g.CanonicalName || Normalize(o.Name) == Normalize(g.CanonicalName)) {
					found = true
					g.CanonicalName = o.Name
					break
				}
			}
			if !found {
				return fmt.Errorf("canonical name %q was not observed", g.CanonicalName)
			}
		}
		for _, id := range g.ObservationIDs {
			if !expected[id] {
				return fmt.Errorf("unknown entity observation %q", id)
			}
			if seen[id] {
				return fmt.Errorf("duplicate entity observation %q", id)
			}
			seen[id] = true
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("entity resolution coverage incomplete")
	}
	return nil
}

// CanonicalizeEntities is an exported operation provided by this package.
// Additional context: A malformed model resolution must not prevent deterministic canonicalization. Fall back to one observed entity per mention.
// Additional context: Model resolutions may split identical names across candidate clusters or chunks. Merge exact normalized canonical names deterministically so the same concept does not become several disconnected nodes.
func CanonicalizeEntities(obs []EntityObservation, candidates EntityCandidates, resolutions []EntityResolution) (CanonicalEntities, error) {
	byID := map[string]EntityObservation{}
	for _, o := range obs {
		byID[o.ID] = o
	}
	out := CanonicalEntities{ObservationToEntity: map[string]string{}}
	add := func(ids []string, name, typ, def string) {
		sort.Strings(ids)
		id := hashID("entity_", strings.Join(ids, "\n"))
		aliases := map[string]bool{}
		for _, oid := range ids {
			aliases[byID[oid].Name] = true
		}
		delete(aliases, name)
		ce := CanonicalEntity{ID: id, CanonicalName: name, Type: typ, Definition: def, ObservationRefs: append([]string(nil), ids...)}
		for a := range aliases {
			ce.Aliases = append(ce.Aliases, a)
		}
		sort.Strings(ce.Aliases)
		out.Entities = append(out.Entities, ce)
		for _, oid := range ids {
			out.ObservationToEntity[oid] = id
		}
	}
	resIndex := 0
	for _, id := range candidates.Singletons {
		o := byID[id]
		add([]string{id}, o.Name, o.TypeHint, o.ContextualDefinition)
	}
	for _, c := range candidates.Candidates {
		var r EntityResolution
		if resIndex < len(resolutions) {
			r = resolutions[resIndex]
		}
		resIndex++
		r = repairEntityResolution(c, r, byID)
		if err := ValidateEntityResolution(c, r, byID); err != nil {

			r = singletonEntityResolution(c, byID)
		}
		for _, g := range r.Groups {
			for _, oid := range g.ObservationIDs {
				if o, ok := byID[oid]; ok && Normalize(o.Name) == Normalize(g.CanonicalName) {
					g.CanonicalName = o.Name
					break
				}
			}
			add(append([]string(nil), g.ObservationIDs...), g.CanonicalName, g.Type, g.Definition)
		}
	}

	merged := map[string]*CanonicalEntity{}
	for _, entity := range out.Entities {
		key := Normalize(entity.CanonicalName)
		current := merged[key]
		if current == nil {
			copyEntity := entity
			copyEntity.Aliases = append([]string(nil), entity.Aliases...)
			copyEntity.ObservationRefs = append([]string(nil), entity.ObservationRefs...)
			merged[key] = &copyEntity
			continue
		}
		current.ObservationRefs = append(current.ObservationRefs, entity.ObservationRefs...)
		if len(entity.Definition) > len(current.Definition) {
			current.Definition = entity.Definition
		}
		if current.Type == "" || current.Type == "Unclassified" {
			current.Type = entity.Type
		}
		current.Aliases = append(current.Aliases, entity.Aliases...)
	}
	out.Entities = out.Entities[:0]
	for _, entity := range merged {
		sort.Strings(entity.ObservationRefs)
		entity.ObservationRefs = uniqueSorted(entity.ObservationRefs)
		entity.ID = hashID("entity_", strings.Join(entity.ObservationRefs, "\n"))
		aliasSet := map[string]bool{}
		for _, alias := range entity.Aliases {
			if Normalize(alias) != Normalize(entity.CanonicalName) {
				aliasSet[alias] = true
			}
		}
		entity.Aliases = entity.Aliases[:0]
		for alias := range aliasSet {
			entity.Aliases = append(entity.Aliases, alias)
		}
		sort.Strings(entity.Aliases)
		out.Entities = append(out.Entities, *entity)
		for _, observationID := range entity.ObservationRefs {
			out.ObservationToEntity[observationID] = entity.ID
		}
	}
	sort.Slice(out.Entities, func(i, j int) bool {
		a, b := Normalize(out.Entities[i].CanonicalName), Normalize(out.Entities[j].CanonicalName)
		if a != b {
			return a < b
		}
		return out.Entities[i].ID < out.Entities[j].ID
	})
	return out, nil
}

// singletonEntityResolution performs the internal operation described by its signature.
func singletonEntityResolution(candidate EntityCandidate, observed map[string]EntityObservation) EntityResolution {
	r := EntityResolution{}
	for _, id := range candidate.Mentions {
		if o, ok := observed[id]; ok {
			r.Groups = append(r.Groups, ResolutionGroup{ObservationIDs: []string{id}, CanonicalName: o.Name, Type: o.TypeHint, Definition: o.ContextualDefinition})
		}
	}
	return r
}

// repairEntityResolution performs the internal operation described by its signature.
func repairEntityResolution(candidate EntityCandidate, resolution EntityResolution, observed map[string]EntityObservation) EntityResolution {
	expected := map[string]bool{}
	for _, id := range candidate.Mentions {
		expected[id] = true
	}
	seen := map[string]bool{}
	repaired := EntityResolution{Reasoning: resolution.Reasoning}
	for _, group := range resolution.Groups {
		ids := make([]string, 0, len(group.ObservationIDs))
		for _, id := range group.ObservationIDs {
			if expected[id] && !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
		if len(ids) == 0 {
			continue
		}
		canonical := group.CanonicalName
		matched := false
		for _, id := range ids {
			if o := observed[id]; Normalize(o.Name) == Normalize(canonical) {
				canonical = o.Name
				matched = true
				break
			}
		}
		if !matched {
			canonical = observed[ids[0]].Name
		}
		repaired.Groups = append(repaired.Groups, ResolutionGroup{ObservationIDs: ids, CanonicalName: canonical, Type: group.Type, Definition: group.Definition})
	}
	for _, id := range candidate.Mentions {
		if !seen[id] {
			if o, ok := observed[id]; ok {
				repaired.Groups = append(repaired.Groups, ResolutionGroup{ObservationIDs: []string{id}, CanonicalName: o.Name, Type: o.TypeHint, Definition: o.ContextualDefinition})
			}
		}
	}
	return repaired
}

// GeneratePredicateCandidates is an exported operation provided by this package.
func GeneratePredicateCandidates(rel []RelationObservation) PredicateCandidates {
	groups := map[string][]int{}
	for i, r := range rel {
		groups[Normalize(r.Predicate)] = append(groups[Normalize(r.Predicate)], i)
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := PredicateCandidates{}
	for _, key := range keys {
		ids := groups[key]
		refs := make([]string, 0, len(ids))
		for _, i := range ids {
			refs = append(refs, rel[i].ID)
		}
		if len(ids) == 1 {
			out.Singletons = append(out.Singletons, PredicateSingleton{Predicate: rel[ids[0]].Predicate, RelationObservationRefs: refs})
		} else {
			preds := make([]string, 0, len(ids))
			for _, i := range ids {
				preds = append(preds, rel[i].Predicate)
			}
			out.Candidates = append(out.Candidates, PredicateCandidate{ID: hashID("predcand_", strings.Join(refs, "\n")), Predicates: uniqueSorted(preds), RelationObservationRefs: refs, Signals: []string{"normalized-equality"}})
		}
	}
	return out
}

// ValidatePredicateResolution is an exported operation provided by this package.
func ValidatePredicateResolution(c PredicateCandidate, r PredicateResolution) error {
	expected := map[string]bool{}
	for _, p := range c.Predicates {
		expected[p] = true
	}
	seen := map[string]bool{}
	for _, g := range r.Groups {
		if g.CanonicalPredicate == "" || !expected[g.CanonicalPredicate] {
			return fmt.Errorf("canonical predicate is not observed")
		}
		for _, p := range g.Predicates {
			if !expected[p] {
				return fmt.Errorf("unknown predicate %q", p)
			}
			if seen[p] {
				return fmt.Errorf("duplicate predicate %q", p)
			}
			seen[p] = true
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("predicate resolution coverage incomplete")
	}
	return nil
}

// BuildGraph is an exported operation provided by this package.
// Additional context: Predicate resolution is optional in the pipeline. When no model step supplied a resolution (or a model returned an unusable one), preserve graph construction with a deterministic fallback: the first observed predicate is canonical and all equivalent candidates map to it.
func BuildGraph(entityObs []EntityObservation, obs []RelationObservation, entities CanonicalEntities, predicates PredicateCandidates, resolutions []PredicateResolution) (Graph, error) {
	predMap := map[string]string{}
	ri := 0
	for _, c := range predicates.Candidates {

		r := fallbackPredicateResolution(c)
		if ri < len(resolutions) {
			candidateResolution := resolutions[ri]
			ri++
			if ValidatePredicateResolution(c, candidateResolution) == nil {
				r = candidateResolution
			}
		}
		for _, g := range r.Groups {
			for _, p := range g.Predicates {
				predMap[p] = g.CanonicalPredicate
			}
		}
	}
	for _, s := range predicates.Singletons {
		predMap[s.Predicate] = s.Predicate
	}
	type key struct{ s, p, t string }
	edges := map[key][]string{}
	unresolved := []UnresolvedRelation{}
	for _, r := range obs {
		source, target := resolveEndpoint(r.Source, r.Provenance.ChunkID, entityObs, entities), resolveEndpoint(r.Target, r.Provenance.ChunkID, entityObs, entities)
		if source == "" {
			unresolved = append(unresolved, UnresolvedRelation{r.ID, "unresolved-source"})
			continue
		}
		if target == "" {
			unresolved = append(unresolved, UnresolvedRelation{r.ID, "unresolved-target"})
			continue
		}
		p := predMap[r.Predicate]
		if p == "" {
			p = r.Predicate
		}
		k := key{source, p, target}
		edges[k] = append(edges[k], r.ID)
	}
	g := Graph{Entities: append([]CanonicalEntity(nil), entities.Entities...), UnresolvedRelations: unresolved}
	keys := make([]key, 0, len(edges))
	for k := range edges {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].s != keys[j].s {
			return keys[i].s < keys[j].s
		}
		if Normalize(keys[i].p) != Normalize(keys[j].p) {
			return Normalize(keys[i].p) < Normalize(keys[j].p)
		}
		return keys[i].t < keys[j].t
	})
	for _, k := range keys {
		refs := uniqueSorted(edges[k])
		g.Edges = append(g.Edges, Edge{ID: hashID("edge_", k.s, k.p, k.t), Source: k.s, Target: k.t, Predicate: k.p, PredicateFamily: k.p, Weight: round4(1 - math.Exp(-float64(len(refs)))), ObservationRefs: refs})
	}
	sort.Slice(g.UnresolvedRelations, func(i, j int) bool {
		return g.UnresolvedRelations[i].RelationObservationID < g.UnresolvedRelations[j].RelationObservationID
	})
	return g, nil
}

// fallbackPredicateResolution performs the internal operation described by its signature.
func fallbackPredicateResolution(c PredicateCandidate) PredicateResolution {
	if len(c.Predicates) == 0 {
		return PredicateResolution{}
	}
	canonical := c.Predicates[0]
	return PredicateResolution{Groups: []PredicateGroup{{
		Predicates:         append([]string(nil), c.Predicates...),
		CanonicalPredicate: canonical,
	}}}
}

// resolveEndpoint performs the internal operation described by its signature.
func resolveEndpoint(name, chunk string, obs []EntityObservation, entities CanonicalEntities) string {
	ids := map[string]bool{}
	n := Normalize(name)
	canonical := make(map[string]CanonicalEntity, len(entities.Entities))
	for _, entity := range entities.Entities {
		canonical[entity.ID] = entity
	}
	for _, o := range obs {
		if o.Provenance.ChunkID != chunk {
			continue
		}
		id := entities.ObservationToEntity[o.ID]
		if id == "" {
			continue
		}
		entity, ok := canonical[id]
		if !ok {
			continue
		}
		matches := Normalize(o.Name) == n || Normalize(entity.CanonicalName) == n
		for _, alias := range entity.Aliases {
			matches = matches || Normalize(alias) == n
		}
		if matches {
			ids[id] = true
		}
	}
	if len(ids) != 1 {
		return ""
	}
	for id := range ids {
		return id
	}
	return ""
}

// round4 performs the internal operation described by its signature.
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

// ValidateGraph is an exported operation provided by this package.
func ValidateGraph(g Graph, entityObs map[string]EntityObservation, relationObs map[string]RelationObservation, docs map[string]bool) error {
	nodes := map[string]bool{}
	for _, n := range g.Entities {
		if nodes[n.ID] {
			return fmt.Errorf("duplicate node ID %s", n.ID)
		}
		nodes[n.ID] = true
		for _, oid := range n.ObservationRefs {
			o, ok := entityObs[oid]
			if !ok || !docs[o.Provenance.DocumentID] || o.Provenance.ChunkID == "" {
				return fmt.Errorf("invalid entity observation reference %s", oid)
			}
		}
	}
	for _, e := range g.Edges {
		if !nodes[e.Source] || !nodes[e.Target] {
			return fmt.Errorf("edge %s references unknown node", e.ID)
		}
		if len(e.ObservationRefs) == 0 {
			return fmt.Errorf("edge %s has no observations", e.ID)
		}
		for _, oid := range e.ObservationRefs {
			if _, ok := relationObs[oid]; !ok {
				return fmt.Errorf("edge %s references unknown relation %s", e.ID, oid)
			}
		}
	}
	for _, u := range g.UnresolvedRelations {
		if _, ok := relationObs[u.RelationObservationID]; !ok {
			return fmt.Errorf("unresolved relation %s is unknown", u.RelationObservationID)
		}
	}
	return nil
}

// Serialize is an exported operation provided by this package.
func Serialize(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// sortedKeys performs the internal operation described by its signature.
func sortedKeys(m map[string]bool) []string {
	r := make([]string, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	sort.Strings(r)
	return r
}

// uniqueSorted performs the internal operation described by its signature.
func uniqueSorted(v []string) []string {
	m := map[string]bool{}
	for _, x := range v {
		m[x] = true
	}
	return sortedKeys(m)
}
