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
type ClassificationResult struct {
	Class         string             `json:"class"`
	Label         string             `json:"label"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

type DecisionCandidateResult struct {
	Candidate   string  `json:"candidate"`
	Value       string  `json:"value"`
	Logprob     float64 `json:"logprob"`
	Probability float64 `json:"probability"`
}

type DecisionResult struct {
	Type              string                    `json:"type"`
	SelectedCandidate string                    `json:"selectedCandidate"`
	SelectedValue     string                    `json:"selectedValue"`
	Confidence        float64                   `json:"confidence"`
	Margin            float64                   `json:"margin"`
	Candidates        []DecisionCandidateResult `json:"candidates"`
}

type classificationToken struct {
	ID    int
	Piece string
}

type tokenizeResponse struct {
	Tokens []json.RawMessage `json:"tokens"`
}

func (c *Client) doClassification(ctx context.Context, req *ChatRequest) (*Interaction, error) {
	return c.doDecision(ctx, req)
}

func (c *Client) doDecision(ctx context.Context, req *ChatRequest) (*Interaction, error) {
	if req == nil || (req.Decision == nil && req.Classification == nil) {
		return nil, fmt.Errorf("decision configuration is required")
	}
	cfg := effectiveDecision(PipelineStep{Decision: req.Decision, Classification: req.Classification})
	if cfg.TopLogprobs == 0 {
		copy := *cfg
		copy.TopLogprobs = 20
		cfg = &copy
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

// extractDecisionScores is the server-response adapter for the current
// OpenAI-compatible logprob response. Decision math does not depend on it.
func extractDecisionScores(validated map[string]classificationToken, position TokenLogprobPosition) (map[string]float64, error) {
	logprobByCandidate := make(map[string]float64, len(validated))
	// llama.cpp versions generally include the sampled token in
	// top_logprobs, but some responses expose it only on the position itself.
	// It is still exact returned probability evidence, so accept it before
	// deciding that a configured candidate is missing.
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

func sortedCandidateKeys(candidates map[string]string) []string {
	keys := make([]string, 0, len(candidates))
	for key := range candidates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

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

func validProbability(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
