package induction

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
)

// systemOneRequest is the TypeSafe-compatible request accepted by llama.cpp's
// native decision endpoint.
type systemOneRequest struct {
	Model     string                       `json:"model"`
	State     any                          `json:"state"`
	Questions map[string]systemOneQuestion `json:"questions"`
}

type systemOneQuestion struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

type systemOneResponse struct {
	Model   string                     `json:"model,omitempty"`
	Answers map[string]systemOneAnswer `json:"answers"`
	Usage   map[string]any             `json:"usage,omitempty"`
}

type systemOneAnswer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice,omitempty"`
	Noul          *float64           `json:"noul,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
}

func (c *Client) doSystemOneDecision(ctx context.Context, req *ChatRequest, cfg *DecisionConfig) (*Interaction, error) {
	if err := validateSystemOneDecisionConfig(cfg); err != nil {
		return nil, err
	}

	state := cfg.State
	if state == nil {
		state = lastUserMessageContent(req.Messages)
	}
	instructions := strings.TrimSpace(cfg.Instructions)
	if instructions == "" {
		instructions = lastUserMessageText(req.Messages)
	}
	if systemInstructions := systemMessageText(req.Messages); systemInstructions != "" {
		instructions = systemInstructions + "\n\n" + instructions
	}
	questionID := strings.TrimSpace(cfg.QuestionID)
	if questionID == "" {
		questionID = "decision"
	}
	questionType := strings.TrimSpace(cfg.Type)
	if questionType == "" {
		questionType = "choice"
	}

	payload := systemOneRequest{
		Model: req.Model,
		State: state,
		Questions: map[string]systemOneQuestion{
			questionID: {
				Type:         questionType,
				Instructions: instructions,
				Criteria:     systemOneCriteria(cfg),
			},
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal systemone request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/v1/systemone", bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.clientHTTP().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("systemone request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read systemone response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("systemone request returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded systemOneResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode systemone response: %w", err)
	}
	answer, ok := decoded.Answers[questionID]
	if !ok {
		return nil, fmt.Errorf("systemone response did not contain answer %q", questionID)
	}
	result, err := buildSystemOneDecisionResult(cfg, answer)
	if err != nil {
		return nil, err
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("serialize systemone decision result: %w", err)
	}
	c.logf("decision: systemone model=%s candidate_count=%d selected_candidate=%s selected_value=%s confidence=%f margin=%f", req.Model, len(cfg.Candidates), result.SelectedCandidate, result.SelectedValue, result.Confidence, result.Margin)
	return &Interaction{Response: string(body), Content: string(resultJSON)}, nil
}

func validateSystemOneDecisionConfig(cfg *DecisionConfig) error {
	if cfg == nil {
		return fmt.Errorf("systemone decision configuration is required")
	}
	questionType := strings.TrimSpace(cfg.Type)
	if questionType == "" {
		questionType = "choice"
	}
	if strings.TrimSpace(cfg.Instructions) == "" && cfg.State == nil {
		return fmt.Errorf("systemone decision requires instructions or a user prompt")
	}
	switch strings.ToLower(questionType) {
	case "choice":
		if strings.TrimSpace(cfg.CandidatesFrom) == "" && len(cfg.Candidates) < 2 {
			return fmt.Errorf("systemone choice requires at least two candidates")
		}
		for candidate, label := range cfg.Candidates {
			if strings.TrimSpace(candidate) == "" || strings.TrimSpace(label) == "" {
				return fmt.Errorf("systemone candidate names and descriptions cannot be empty")
			}
		}
	case "noul":
		criteria, ok := systemOneCriteria(cfg).(map[string]string)
		if !ok || strings.TrimSpace(criteria["true"]) == "" || strings.TrimSpace(criteria["false"]) == "" {
			return fmt.Errorf("systemone noul requires true and false criteria")
		}
	case "score":
		criteria, ok := systemOneCriteria(cfg).([]string)
		if !ok || len(criteria) < 2 || len(criteria) > 10 {
			return fmt.Errorf("systemone score requires 2 to 10 ordered criteria")
		}
		for _, level := range criteria {
			if strings.TrimSpace(level) == "" {
				return fmt.Errorf("systemone score criteria cannot be empty")
			}
		}
	default:
		return fmt.Errorf("systemone decision type %q is unsupported", cfg.Type)
	}
	return nil
}

func buildSystemOneDecisionResult(cfg *DecisionConfig, answer systemOneAnswer) (*DecisionResult, error) {
	questionType := strings.ToLower(strings.TrimSpace(cfg.Type))
	if questionType == "" {
		questionType = "choice"
	}
	if answer.Type != "" && !strings.EqualFold(answer.Type, questionType) {
		return nil, fmt.Errorf("systemone response returned answer type %q, want %s", answer.Type, questionType)
	}
	if questionType == "noul" {
		return buildSystemOneNoulResult(answer)
	}
	if questionType == "score" {
		return buildSystemOneScoreResult(answer)
	}
	if len(answer.Probabilities) == 0 {
		return nil, fmt.Errorf("systemone response did not include choice probabilities")
	}
	keys := sortedCandidateKeys(cfg.Candidates)
	for _, candidate := range keys {
		probability, ok := answer.Probabilities[candidate]
		if !ok || math.IsNaN(probability) || math.IsInf(probability, 0) || probability < 0 {
			return nil, fmt.Errorf("systemone response did not include a valid probability for candidate %q", candidate)
		}
	}
	winner := strings.TrimSpace(answer.Choice)
	if _, ok := cfg.Candidates[winner]; !ok {
		return nil, fmt.Errorf("systemone response selected unknown candidate %q", answer.Choice)
	}
	second := 0.0
	sum := 0.0
	rows := make([]DecisionCandidateResult, 0, len(keys))
	for _, candidate := range keys {
		probability := answer.Probabilities[candidate]
		sum += probability
		if candidate != winner && probability > second {
			second = probability
		}
		// System One criteria keys are already the semantic values consumed by
		// pipeline `when` clauses. The descriptions remain model-facing text.
		rows = append(rows, DecisionCandidateResult{Candidate: candidate, Value: candidate, Probability: probability})
	}
	if math.IsNaN(sum) || math.IsInf(sum, 0) || sum <= 0 || math.Abs(sum-1) > 1e-4 {
		return nil, fmt.Errorf("systemone choice probabilities must sum to 1, got %f", sum)
	}
	return &DecisionResult{
		Type:              "decision",
		SelectedCandidate: winner,
		SelectedValue:     winner,
		Confidence:        answer.Probabilities[winner],
		Margin:            answer.Probabilities[winner] - second,
		Candidates:        rows,
	}, nil
}

func systemOneCriteria(cfg *DecisionConfig) any {
	if cfg.Criteria != nil {
		switch value := cfg.Criteria.(type) {
		case map[string]string:
			return value
		case map[string]any:
			criteria := make(map[string]string, len(value))
			for key, item := range value {
				if text, ok := item.(string); ok {
					criteria[key] = text
				}
			}
			return criteria
		case []any:
			criteria := make([]string, 0, len(value))
			for _, item := range value {
				if text, ok := item.(string); ok {
					criteria = append(criteria, text)
				}
			}
			return criteria
		default:
			return cfg.Criteria
		}
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Type), "noul") {
		return map[string]string{"true": cfg.Candidates["true"], "false": cfg.Candidates["false"]}
	}
	return cfg.Candidates
}

func buildSystemOneNoulResult(answer systemOneAnswer) (*DecisionResult, error) {
	if answer.Noul == nil || math.IsNaN(*answer.Noul) || math.IsInf(*answer.Noul, 0) || *answer.Noul < 0 || *answer.Noul > 1 {
		return nil, fmt.Errorf("systemone noul response did not include a valid noul probability")
	}
	p := *answer.Noul
	winner, confidence := "false", 1-p
	if p >= 0.5 {
		winner, confidence = "true", p
	}
	return &DecisionResult{Type: "decision", SelectedCandidate: winner, SelectedValue: winner, Confidence: confidence, Margin: math.Abs(2*p - 1), Candidates: []DecisionCandidateResult{
		{Candidate: "false", Value: "false", Probability: 1 - p},
		{Candidate: "true", Value: "true", Probability: p},
	}}, nil
}

func buildSystemOneScoreResult(answer systemOneAnswer) (*DecisionResult, error) {
	if answer.Score == nil || math.IsNaN(*answer.Score) || math.IsInf(*answer.Score, 0) {
		return nil, fmt.Errorf("systemone score response did not include a valid score")
	}
	if len(answer.Probabilities) == 0 {
		return nil, fmt.Errorf("systemone score response did not include probabilities")
	}
	rows := make([]DecisionCandidateResult, 0, len(answer.Probabilities))
	maxProbability := 0.0
	for level, probability := range answer.Probabilities {
		if math.IsNaN(probability) || math.IsInf(probability, 0) || probability < 0 {
			return nil, fmt.Errorf("systemone score response returned invalid probability for level %q", level)
		}
		if probability > maxProbability {
			maxProbability = probability
		}
		rows = append(rows, DecisionCandidateResult{Candidate: level, Value: answer.Legend[level], Probability: probability})
	}
	return &DecisionResult{Type: "decision", SelectedCandidate: "score", SelectedValue: fmt.Sprintf("%.6g", *answer.Score), Confidence: maxProbability, Candidates: rows}, nil
}

func lastUserMessageContent(messages []Message) any {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && messages[i].Content != nil {
			return messages[i].Content
		}
	}
	return nil
}

func lastUserMessageText(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		if content, ok := messages[i].Content.(string); ok {
			return content
		}
	}
	return ""
}

func systemMessageText(messages []Message) string {
	parts := make([]string, 0, 1)
	for _, message := range messages {
		if message.Role != "system" {
			continue
		}
		if content, ok := message.Content.(string); ok && strings.TrimSpace(content) != "" {
			parts = append(parts, strings.TrimSpace(content))
		}
	}
	return strings.Join(parts, "\n\n")
}
