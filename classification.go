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

type classificationToken struct {
	ID    int
	Piece string
}

type tokenizeResponse struct {
	Tokens []json.RawMessage `json:"tokens"`
}

func (c *Client) doClassification(ctx context.Context, req *ChatRequest) (*Interaction, error) {
	if req == nil || req.Classification == nil {
		return nil, fmt.Errorf("classification configuration is required")
	}
	cfg := req.Classification
	if cfg.TopLogprobs == 0 {
		copy := *cfg
		copy.TopLogprobs = 20
		cfg = &copy
	}
	if err := validateClassificationConfig(cfg); err != nil {
		return nil, err
	}
	validated, err := c.validateClassificationCandidates(ctx, req.Model, cfg)
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
	payload, err := json.Marshal(&request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal classification request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.clientHTTP().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("classification request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read classification response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("classification request returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded InferenceResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("failed to decode classification response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return nil, fmt.Errorf("classification failed: llama.cpp response contained no choices")
	}
	choice := decoded.Choices[0]
	if choice.Logprobs == nil {
		return nil, fmt.Errorf("classification failed: llama.cpp response did not contain logprobs")
	}
	if len(choice.Logprobs.Content) == 0 {
		return nil, fmt.Errorf("classification failed: llama.cpp returned no next-token probability data")
	}

	logprobByCandidate := make(map[string]float64, len(validated))
	position := choice.Logprobs.Content[0]
	// llama.cpp versions generally include the sampled token in
	// top_logprobs, but some responses expose it only on the position itself.
	// It is still exact returned probability evidence, so accept it before
	// deciding that a configured candidate is missing.
	for candidate, token := range validated {
		if position.Token == token.Piece {
			if !validProbability(position.Logprob) {
				return nil, fmt.Errorf("classification failed: candidate %q returned an invalid log probability", candidate)
			}
			logprobByCandidate[candidate] = position.Logprob
		}
	}
	for _, top := range position.TopLogprobs {
		for candidate, token := range validated {
			if top.Token == token.Piece {
				if !validProbability(top.Logprob) {
					return nil, fmt.Errorf("classification failed: candidate %q returned an invalid log probability", candidate)
				}
				logprobByCandidate[candidate] = top.Logprob
			}
		}
	}
	for candidate := range validated {
		if _, ok := logprobByCandidate[candidate]; !ok {
			return nil, fmt.Errorf("classification failed: candidate %q was not present in top_logprobs=%d; increase classification.topLogprobs or choose candidate tokens that the model reliably associates with the prompt", candidate, cfg.TopLogprobs)
		}
	}

	candidates := make([]string, 0, len(logprobByCandidate))
	for candidate := range logprobByCandidate {
		candidates = append(candidates, candidate)
	}
	sort.Strings(candidates)
	maxLogprob := math.Inf(-1)
	for _, candidate := range candidates {
		if logprobByCandidate[candidate] > maxLogprob {
			maxLogprob = logprobByCandidate[candidate]
		}
	}
	probabilities := make(map[string]float64, len(candidates))
	denominator := 0.0
	for _, candidate := range candidates {
		value := math.Exp(logprobByCandidate[candidate] - maxLogprob)
		if !validProbability(value) {
			return nil, fmt.Errorf("classification failed: candidate %q produced an invalid normalized probability", candidate)
		}
		probabilities[candidate] = value
		denominator += value
	}
	if !validProbability(denominator) || denominator <= 0 {
		return nil, fmt.Errorf("classification failed: candidate probability denominator was invalid")
	}

	winner := candidates[0]
	sum := 0.0
	for _, candidate := range candidates {
		probabilities[candidate] /= denominator
		sum += probabilities[candidate]
		if probabilities[candidate] > probabilities[winner] {
			winner = candidate
		}
	}
	if !validProbability(sum) || math.Abs(sum-1) > 1e-9 {
		return nil, fmt.Errorf("classification failed: normalized probabilities did not sum to 1")
	}
	result := ClassificationResult{Class: winner, Label: cfg.Candidates[winner], Confidence: probabilities[winner], Probabilities: probabilities}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("serialize classification result: %w", err)
	}
	c.logf("classification: model=%s candidate_count=%d top_logprobs=%d selected_class=%s selected_label=%s confidence=%f", req.Model, len(candidates), cfg.TopLogprobs, result.Class, result.Label, result.Confidence)
	return &Interaction{Response: string(body), Content: string(encoded)}, nil
}

func validateClassificationConfig(cfg *ClassificationConfig) error {
	if cfg == nil || len(cfg.Candidates) < 2 {
		return fmt.Errorf("classification requires at least two candidates")
	}
	if cfg.TopLogprobs <= 0 {
		return fmt.Errorf("classification topLogprobs must be positive")
	}
	if cfg.TopLogprobs < len(cfg.Candidates) {
		return fmt.Errorf("classification topLogprobs must be at least the number of candidates")
	}
	for candidate, label := range cfg.Candidates {
		if strings.TrimSpace(candidate) == "" {
			return fmt.Errorf("classification candidate key cannot be empty")
		}
		if strings.TrimSpace(label) == "" {
			return fmt.Errorf("classification label for candidate %q cannot be empty", candidate)
		}
	}
	return nil
}

func (c *Client) validateClassificationCandidates(ctx context.Context, model string, cfg *ClassificationConfig) (map[string]classificationToken, error) {
	if c.classificationTokens == nil {
		c.classificationTokens = &sync.Map{}
	}
	result := make(map[string]classificationToken, len(cfg.Candidates))
	candidates := make([]string, 0, len(cfg.Candidates))
	for candidate := range cfg.Candidates {
		candidates = append(candidates, candidate)
	}
	sort.Strings(candidates)
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
		return classificationToken{}, fmt.Errorf("classification candidate %q tokenization failed: %w", candidate, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return classificationToken{}, fmt.Errorf("read tokenization response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return classificationToken{}, fmt.Errorf("classification candidate %q tokenization returned %d: %s", candidate, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded tokenizeResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return classificationToken{}, fmt.Errorf("decode tokenization response: %w", err)
	}
	if len(decoded.Tokens) != 1 {
		return classificationToken{}, fmt.Errorf("classification candidate %q tokenizes to %d tokens; candidates must be exactly one token", candidate, len(decoded.Tokens))
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
		return classificationToken{}, fmt.Errorf("classification candidate %q tokenization did not return a token piece", candidate)
	}
	return classificationToken{ID: id, Piece: candidate}, nil
}

func validProbability(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
