// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// ClassificationResult is the application-generated result of a bounded
// next-token classification request. Confidence is conditional on the
// configured candidates and returned candidate log probabilities; it is not a
// calibrated real-world probability.
// ClassificationResult is an exported type in this package's public data model.
type ClassificationResult struct {
	// Class stores the value represented by this member.
	Class string `json:"class"`
	// Label stores the value represented by this member.
	Label string `json:"label"`
	// Confidence stores the value represented by this member.
	Confidence float64 `json:"confidence"`
	// Probabilities stores the value represented by this member.
	Probabilities map[string]float64 `json:"probabilities"`
}

// DecisionCandidateResult is an exported type in this package's public data model.
type DecisionCandidateResult struct {
	// Candidate stores the value represented by this member.
	Candidate string `json:"candidate"`
	// Value stores the value represented by this member.
	Value string `json:"value"`
	// Logprob stores the value represented by this member.
	Logprob float64 `json:"logprob"`
	// Probability stores the value represented by this member.
	Probability float64 `json:"probability"`
}

// DecisionResult is an exported type in this package's public data model.
type DecisionResult struct {
	// Type stores the value represented by this member.
	Type string `json:"type"`
	// SelectedCandidate stores the value represented by this member.
	SelectedCandidate string `json:"selectedCandidate"`
	// SelectedValue stores the value represented by this member.
	SelectedValue string `json:"selectedValue"`
	// Confidence stores the value represented by this member.
	Confidence float64 `json:"confidence"`
	// Margin stores the value represented by this member.
	Margin float64 `json:"margin"`
	// Candidates stores the value represented by this member.
	Candidates []DecisionCandidateResult `json:"candidates"`
}

// classificationToken is an internal data type used by this package.
type classificationToken struct {
	// ID stores the value represented by this member.
	ID int
	// Piece stores the value represented by this member.
	Piece string
}

// tokenizeResponse is an internal data type used by this package.
type tokenizeResponse struct {
	// Tokens stores the value represented by this member.
	Tokens []json.RawMessage `json:"tokens"`
}

// doClassification performs the internal operation described by its signature.
func (c *Client) doClassification(ctx context.Context, req *ChatRequest) (*Interaction, error) {
	return c.doDecision(ctx, req)
}

// doDecision performs the internal operation described by its signature.
func (c *Client) doDecision(ctx context.Context, req *ChatRequest) (*Interaction, error) {
	if req == nil || (req.Decision == nil && req.Classification == nil) {
		return nil, fmt.Errorf("decision configuration is required")
	}
	if err := c.validateDecisionModel(ctx, req.Model); err != nil {
		return nil, err
	}
	cfg := effectiveDecision(PipelineStep{Decision: req.Decision, Classification: req.Classification})
	if strings.EqualFold(strings.TrimSpace(cfg.Endpoint), "systemone") {
		return c.doSystemOneDecision(ctx, req, cfg)
	}
	if cfg.TopLogprobs == 0 {
		defaultConfig := *cfg
		defaultConfig.TopLogprobs = 20
		cfg = &defaultConfig
	}
	if err := validateDecisionConfig(cfg); err != nil {
		return nil, err
	}
	validated, err := c.validateDecisionCandidates(ctx, req.Model, cfg)
	if err != nil {
		return nil, err
	}

	request := *req
	maxTokens, stream, logprobs, topLogprobs := 1, false, true, cfg.TopLogprobs
	request.MaxTokens = &maxTokens
	request.Stream = &stream
	request.Logprobs = &logprobs
	request.TopLogprobs = &topLogprobs
	request.ChatTemplateKwargs = map[string]any{"enable_thinking": false}
	request.Classification = nil
	request.Decision = nil
	payload, err := json.Marshal(&request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal decision request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.clientHTTP().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("decision request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read decision response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("decision request returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded InferenceResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("failed to decode decision response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return nil, fmt.Errorf("decision failed: llama.cpp response contained no choices")
	}
	choice := decoded.Choices[0]
	if choice.Logprobs == nil {
		return nil, fmt.Errorf("decision failed: llama.cpp response did not contain logprobs")
	}
	if len(choice.Logprobs.Content) == 0 {
		return nil, fmt.Errorf("decision failed: llama.cpp returned no next-token probability data")
	}

	position := choice.Logprobs.Content[0]
	logprobByCandidate, err := extractDecisionScores(validated, position)
	if err != nil {
		return nil, err
	}
	result, err := buildDecisionResult(cfg, logprobByCandidate)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("serialize decision result: %w", err)
	}
	c.logf("decision: model=%s candidate_count=%d top_logprobs=%d selected_candidate=%s selected_value=%s confidence=%f margin=%f", req.Model, len(cfg.Candidates), cfg.TopLogprobs, result.SelectedCandidate, result.SelectedValue, result.Confidence, result.Margin)
	return &Interaction{Response: string(body), Content: string(encoded)}, nil
}

// validateDecisionModel requires the exact requested model to advertise the
// native decision output modality in the server's authoritative model catalog.
func (c *Client) validateDecisionModel(ctx context.Context, model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("decision model is required")
	}
	records, err := c.fetchModelRecordsAt(ctx, "/v1/models", false)
	if err != nil {
		return fmt.Errorf("validate decision model %q: %w", model, err)
	}
	entry, ok := runtimeModelByID(records, model)
	if !ok {
		return fmt.Errorf("decision model %q was not found in /v1/models", model)
	}
	if !hasOutputMode(entry.OutputModes, "decisions") {
		return fmt.Errorf("model %q cannot be used for decisions: /v1/models does not advertise output modality %q", model, "decisions")
	}
	return nil
}

// extractDecisionScores is the server-response adapter for the current
// OpenAI-compatible logprob response. Decision math does not depend on it.
// Additional context: llama.cpp versions generally include the sampled token in top_logprobs, but some responses expose it only on the position itself. It is still exact returned probability evidence, so accept it before deciding that a configured candidate is missing.
func extractDecisionScores(validated map[string]classificationToken, position TokenLogprobPosition) (map[string]float64, error) {
	logprobByCandidate := make(map[string]float64, len(validated))

	for candidate, token := range validated {
		if position.Token == token.Piece {
			if !validProbability(position.Logprob) {
				return nil, fmt.Errorf("decision failed: candidate %q returned an invalid log probability", candidate)
			}
			logprobByCandidate[candidate] = position.Logprob
		}
	}
	for _, top := range position.TopLogprobs {
		for candidate, token := range validated {
			if top.Token == token.Piece {
				if !validProbability(top.Logprob) {
					return nil, fmt.Errorf("decision failed: candidate %q returned an invalid log probability", candidate)
				}
				logprobByCandidate[candidate] = top.Logprob
			}
		}
	}
	return logprobByCandidate, nil
}

// buildDecisionResult is independent of the server response format so a future
// score-only endpoint can feed the same deterministic normalization path.
func buildDecisionResult(cfg *DecisionConfig, logprobByCandidate map[string]float64) (*DecisionResult, error) {
	if err := validateDecisionConfig(cfg); err != nil {
		return nil, err
	}
	keys := sortedCandidateKeys(cfg.Candidates)
	maxLogprob := math.Inf(-1)
	for _, candidate := range keys {
		logprob, ok := logprobByCandidate[candidate]
		if !ok {
			return nil, fmt.Errorf("decision response did not include candidate %q; increase decision.topLogprobs", candidate)
		}
		if !validProbability(logprob) {
			return nil, fmt.Errorf("decision candidate %q returned an invalid log probability", candidate)
		}
		if logprob > maxLogprob {
			maxLogprob = logprob
		}
	}
	weights := make(map[string]float64, len(keys))
	denominator := 0.0
	for _, candidate := range keys {
		weight := math.Exp(logprobByCandidate[candidate] - maxLogprob)
		if !validProbability(weight) {
			return nil, fmt.Errorf("decision candidate %q produced an invalid normalized probability", candidate)
		}
		weights[candidate] = weight
		denominator += weight
	}
	if !validProbability(denominator) || denominator <= 0 {
		return nil, fmt.Errorf("decision candidate probability denominator was invalid")
	}
	probabilities := make(map[string]float64, len(keys))
	winner := keys[0]
	for _, candidate := range keys {
		probabilities[candidate] = weights[candidate] / denominator
		if probabilities[candidate] > probabilities[winner] {
			winner = candidate
		}
	}
	second := 0.0
	rows := make([]DecisionCandidateResult, 0, len(keys))
	sum := 0.0
	for _, candidate := range keys {
		probability := probabilities[candidate]
		if candidate != winner && probability > second {
			second = probability
		}
		sum += probability
		rows = append(rows, DecisionCandidateResult{Candidate: candidate, Value: cfg.Candidates[candidate], Logprob: logprobByCandidate[candidate], Probability: probability})
	}
	if !validProbability(sum) || math.Abs(sum-1) > 1e-9 {
		return nil, fmt.Errorf("decision normalized probabilities did not sum to 1")
	}
	return &DecisionResult{Type: "decision", SelectedCandidate: winner, SelectedValue: cfg.Candidates[winner], Confidence: probabilities[winner], Margin: probabilities[winner] - second, Candidates: rows}, nil
}

// validateDecisionConfig performs the internal operation described by its signature.
func validateDecisionConfig(cfg *DecisionConfig) error {
	if cfg == nil || len(cfg.Candidates) < 2 {
		return fmt.Errorf("decision requires at least two candidates")
	}
	if cfg.TopLogprobs <= 0 {
		return fmt.Errorf("decision topLogprobs must be positive")
	}
	if cfg.TopLogprobs < len(cfg.Candidates) {
		return fmt.Errorf("decision topLogprobs must be at least the number of candidates")
	}
	for candidate, label := range cfg.Candidates {
		if strings.TrimSpace(candidate) == "" {
			return fmt.Errorf("decision candidate key cannot be empty")
		}
		if strings.TrimSpace(label) == "" {
			return fmt.Errorf("decision value for candidate %q cannot be empty", candidate)
		}
	}
	return nil
}

// sortedCandidateKeys performs the internal operation described by its signature.
func sortedCandidateKeys(candidates map[string]string) []string {
	keys := make([]string, 0, len(candidates))
	for key := range candidates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// validateDecisionCandidates performs the internal operation described by its signature.
func (c *Client) validateDecisionCandidates(ctx context.Context, model string, cfg *DecisionConfig) (map[string]classificationToken, error) {
	if c.classificationTokens == nil {
		c.classificationTokens = &sync.Map{}
	}
	result := make(map[string]classificationToken, len(cfg.Candidates))
	candidates := sortedCandidateKeys(cfg.Candidates)
	for _, candidate := range candidates {
		cacheKey := model + "\x00" + candidate
		if cached, ok := c.classificationTokens.Load(cacheKey); ok {
			result[candidate] = cached.(classificationToken)
			continue
		}
		token, err := c.tokenizeClassificationCandidate(ctx, model, candidate)
		if err != nil {
			return nil, err
		}
		c.classificationTokens.Store(cacheKey, token)
		result[candidate] = token
	}
	return result, nil
}

// tokenizeClassificationCandidate performs the internal operation described by its signature.
func (c *Client) tokenizeClassificationCandidate(ctx context.Context, model, candidate string) (classificationToken, error) {
	payload, err := json.Marshal(map[string]any{"model": model, "content": candidate, "add_special": false, "parse_special": true, "with_pieces": true})
	if err != nil {
		return classificationToken{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/tokenize", bytes.NewReader(payload))
	if err != nil {
		return classificationToken{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.clientHTTP().Do(req)
	if err != nil {
		return classificationToken{}, fmt.Errorf("decision candidate %q tokenization failed: %w", candidate, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return classificationToken{}, fmt.Errorf("read tokenization response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return classificationToken{}, fmt.Errorf("decision candidate %q tokenization returned %d: %s", candidate, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded tokenizeResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return classificationToken{}, fmt.Errorf("decode tokenization response: %w", err)
	}
	if len(decoded.Tokens) != 1 {
		return classificationToken{}, fmt.Errorf("decision candidate %q tokenizes to %d tokens; candidates must be exactly one token", candidate, len(decoded.Tokens))
	}
	var object struct {
		ID    int    `json:"id"`
		Piece string `json:"piece"`
	}
	if err := json.Unmarshal(decoded.Tokens[0], &object); err == nil && object.Piece != "" {
		return classificationToken{ID: object.ID, Piece: object.Piece}, nil
	}
	var id int
	if err := json.Unmarshal(decoded.Tokens[0], &id); err != nil {
		return classificationToken{}, fmt.Errorf("decision candidate %q tokenization did not return a token piece", candidate)
	}
	return classificationToken{ID: id, Piece: candidate}, nil
}

// validProbability performs the internal operation described by its signature.
func validProbability(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
