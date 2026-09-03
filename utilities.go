package induction

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// healthCheckTimeout bounds the HTTP request duration used by CheckHealth.
const healthCheckTimeout = 10 * time.Second

// ListLoadedModels fetches /v1/models and logs only loaded models.
func ListLoadedModels(endpoint string, options ...ClientOption) error {
	client := NewClient(context.Background(), endpoint, options...)
	return client.ListLoadedModels()
}

// ListLoadedModels fetches /v1/models and sends the loaded-model table to the
// configured logger.
func (c *Client) ListLoadedModels() error {
	ctx, cancel := context.WithTimeout(context.Background(), listModelsTimeout)
	defer cancel()

	rows, err := c.fetchModelRows(ctx)
	if err != nil {
		return err
	}

	var table strings.Builder
	renderModelTable(&table, filterLoadedModelRows(rows))
	c.logTable(table.String())
	return nil
}

// CheckHealth probes the server health endpoint for the provided endpoint.
func CheckHealth(endpoint string, options ...ClientOption) error {
	client := NewClient(context.Background(), endpoint, options...)
	return client.CheckHealth()
}

// CheckHealth probes the server health endpoints for the client endpoint.
func (c *Client) CheckHealth() error {
	ctx, cancel := context.WithTimeout(context.Background(), healthCheckTimeout)
	defer cancel()

	var lastErr error
	for _, path := range []string{"/health", "/v1/health"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+path, nil)
		if err != nil {
			return err
		}

		resp, err := c.clientHTTP().Do(req)
		if err != nil {
			return fmt.Errorf("health request failed: %w", err)
		}

		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			return nil
		}

		lastErr = fmt.Errorf("health endpoint %s returned %d", path, resp.StatusCode)
	}

	return lastErr
}

// GenerateSnapshot fetches telemetry for the requested model using a convenience client.
func GenerateSnapshot(ctx context.Context, endpoint string, req *ChatRequest, options ...ClientOption) (*ModelSnapshot, error) {
	client := NewClient(ctx, endpoint, options...)
	return client.GenerateSnapshot(ctx, req)
}

// InferSnapshot loads induction.yaml from the current working directory,
// applies its model and timeout, and runs inference with telemetry collection.
func InferSnapshot(ctx context.Context, req *ChatRequest, options ...ClientOption) (*ModelSnapshot, error) {
	client, request, inferenceCtx, cancel, err := configuredInference(ctx, req, options...)
	if err != nil {
		return nil, err
	}
	defer cancel()
	snapshot, err := client.GenerateSnapshot(inferenceCtx, request)
	if err != nil {
		return nil, err
	}
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	if cfg.PersistSnapshots {
		session, err := newSnapshotSession(request.Model)
		if err != nil {
			return nil, err
		}
		if err := session.save([]*ModelSnapshot{snapshot}); err != nil {
			return nil, err
		}
	}
	return snapshot, nil
}

// Infer runs a standard OpenAI-compatible inference request using the model,
// server, and timeout configured in induction.yaml.
func Infer(ctx context.Context, req *ChatRequest, options ...ClientOption) (*InferenceResponse, error) {
	client, request, inferenceCtx, cancel, err := configuredInference(ctx, req, options...)
	if err != nil {
		return nil, err
	}
	defer cancel()
	monitor := client.startInferenceMonitor(inferenceCtx, request.Model, false)
	defer monitor.Stop()
	response, err := client.infer(inferenceCtx, request)
	if err == nil && monitor.overlay != nil {
		monitor.overlay.Complete()
	}
	return response, err
}

// InferStreamChunks runs a streaming inference request and calls yield for each
// typed OpenAI-compatible chunk object. SSE framing is consumed internally.
func InferStreamChunks(ctx context.Context, req *ChatRequest, yield func(InferenceStreamChunk) error, options ...ClientOption) error {
	if yield == nil {
		return fmt.Errorf("chunk handler is nil")
	}
	client, request, inferenceCtx, cancel, err := configuredInference(ctx, req, options...)
	if err != nil {
		return err
	}
	defer cancel()
	monitor := client.startInferenceMonitor(inferenceCtx, request.Model, false)
	defer monitor.Stop()
	err = client.inferStreamChunks(inferenceCtx, request, yield)
	if err == nil && monitor.overlay != nil {
		monitor.overlay.Complete()
	}
	return err
}

// InferStream runs a streaming inference request and writes only generated
// content to out, suitable for displaying directly in a chat interface.
func InferStream(ctx context.Context, req *ChatRequest, out io.Writer, options ...ClientOption) error {
	if out == nil {
		return fmt.Errorf("output writer is nil")
	}
	return renderInferenceStream(out, func(yield func(InferenceStreamChunk) error) error {
		return InferStreamChunks(ctx, req, yield, options...)
	})
}

func renderInferenceStream(out io.Writer, stream func(func(InferenceStreamChunk) error) error) error {
	reasoningOpen := false
	write := func(content string) error {
		if _, err := io.WriteString(out, content); err != nil {
			return fmt.Errorf("write stream content: %w", err)
		}
		return nil
	}
	err := stream(func(chunk InferenceStreamChunk) error {
		for _, choice := range chunk.Choices {
			if reasoning := choice.Delta.ReasoningContent; reasoning != "" {
				if !reasoningOpen {
					if err := write("<think>\n"); err != nil {
						return err
					}
					reasoningOpen = true
				}
				if err := write(reasoning); err != nil {
					return err
				}
			}
			content := choice.Delta.Content
			if content == "" {
				content = choice.Text
			}
			if content != "" {
				if reasoningOpen {
					if err := write("\n</think>\n\n"); err != nil {
						return err
					}
					reasoningOpen = false
				}
				if err := write(content); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if reasoningOpen {
		return write("\n</think>")
	}
	return nil
}

func configuredInference(ctx context.Context, req *ChatRequest, options ...ClientOption) (*Client, *ChatRequest, context.Context, context.CancelFunc, error) {
	if req == nil {
		return nil, nil, nil, nil, fmt.Errorf("request is nil")
	}
	cfg, err := LoadConfig()
	if err != nil {
		return nil, nil, nil, nil, err
	}

	request := *req
	if request.Model == "" {
		return nil, nil, nil, nil, fmt.Errorf("request model is required")
	}
	inferenceCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Timeout))
	client := newClientFromConfig(inferenceCtx, cfg, options...)
	return client, &request, inferenceCtx, cancel, nil
}

// Chat sends a chat-completion request using a convenience client.
func Chat(ctx context.Context, endpoint string, req *ChatRequest, options ...ClientOption) (*Interaction, error) {
	client := NewClient(ctx, endpoint, options...)
	return client.Chat(ctx, req)
}

// Complete sends a completion request using a convenience client.
func Complete(ctx context.Context, endpoint string, req *ChatRequest, options ...ClientOption) (*Interaction, error) {
	client := NewClient(ctx, endpoint, options...)
	return client.Complete(ctx, req)
}

// StreamChat sends a streaming chat-completion request and writes streamed text to out.
func StreamChat(ctx context.Context, endpoint string, req *ChatRequest, out io.Writer, options ...ClientOption) (*Interaction, error) {
	client := NewClient(ctx, endpoint, options...)
	return client.StreamChat(ctx, req, out)
}

// StreamComplete sends a streaming completion request and writes streamed text to out.
func StreamComplete(ctx context.Context, endpoint string, req *ChatRequest, out io.Writer, options ...ClientOption) (*Interaction, error) {
	client := NewClient(ctx, endpoint, options...)
	return client.StreamComplete(ctx, req, out)
}

// fetchModelRows retrieves and normalizes the model listing response.
func (c *Client) fetchModelRows(ctx context.Context) ([]modelRow, error) {
	records, err := c.fetchModelRecordsAt(ctx, "/v1/models", false)
	if err != nil {
		return nil, err
	}
	rows := make([]modelRow, 0, len(records))
	for _, record := range records {
		rows = append(rows, modelRowFromMap(record))
	}
	return rows, nil
}

// filterLoadedModelRows returns only the rows flagged as loaded.
func filterLoadedModelRows(rows []modelRow) []modelRow {
	if len(rows) == 0 {
		return nil
	}

	loaded := make([]modelRow, 0, len(rows))
	for _, row := range rows {
		if row.Loaded {
			loaded = append(loaded, row)
		}
	}
	return loaded
}

// Chat runs a chat-completion request against the explicit chat endpoint.
func (c *Client) Chat(ctx context.Context, req *ChatRequest) (*Interaction, error) {
	return c.doInferenceAt(ctx, req, "/v1/chat/completions")
}

// Complete runs a plain completion request against the explicit completion endpoint.
func (c *Client) Complete(ctx context.Context, req *ChatRequest) (*Interaction, error) {
	return c.doInferenceAt(ctx, req, "/completion")
}

// StreamChat runs a streaming chat-completion request and writes the streamed text to out.
func (c *Client) StreamChat(ctx context.Context, req *ChatRequest, out io.Writer) (*Interaction, error) {
	return c.doStreamInferenceAt(ctx, req, "/v1/chat/completions", out)
}

// StreamComplete runs a streaming completion request and writes the streamed text to out.
func (c *Client) StreamComplete(ctx context.Context, req *ChatRequest, out io.Writer) (*Interaction, error) {
	return c.doStreamInferenceAt(ctx, req, "/completion", out)
}

// doInferenceAt posts a completion payload to the supplied endpoint path and returns the parsed interaction.
func (c *Client) doInferenceAt(ctx context.Context, req *ChatRequest, path string) (*Interaction, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+path, bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.clientHTTP().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("inference request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read inference body: %w", err)
	}

	interaction := &Interaction{Response: string(bodyBytes)}
	interaction.Content = extractInteractionContent(bodyBytes)
	interaction.ReasoningContent = extractInteractionReasoningContent(bodyBytes)
	return interaction, nil
}

// doStreamInferenceAt posts a streaming completion payload to the supplied endpoint path and collects the streamed text.
func (c *Client) doStreamInferenceAt(ctx context.Context, req *ChatRequest, path string, out io.Writer) (*Interaction, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if out == nil {
		out = io.Discard
	}

	cloned := *req
	stream := true
	cloned.Stream = &stream

	payload, err := json.Marshal(&cloned)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+path, bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.clientHTTP().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("streaming request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var raw strings.Builder
	var content strings.Builder
	reader := bufio.NewReader(resp.Body)

	for {
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			raw.WriteString(line)
			chunk := streamedContent(line)
			if chunk == doneStreamToken {
				break
			}
			if chunk != "" {
				content.WriteString(chunk)
				if _, err := io.WriteString(out, chunk); err != nil {
					return nil, fmt.Errorf("failed to write stream content: %w", err)
				}
			}
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("failed to read stream body: %w", readErr)
		}
	}

	return &Interaction{Content: content.String(), Response: raw.String()}, nil
}

// doneStreamToken marks the end of an SSE-style stream.
const doneStreamToken = "[DONE]"

// streamedContent extracts a text chunk from a streamed response line.
func streamedContent(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}

	if strings.HasPrefix(trimmed, "data:") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
	}
	if trimmed == doneStreamToken {
		return doneStreamToken
	}

	if chunk := contentFromStreamJSON(trimmed); chunk != "" {
		return chunk
	}

	return trimmed
}

// contentFromStreamJSON extracts assistant text from a llama.cpp-style streaming JSON payload.
func contentFromStreamJSON(payload string) string {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		return ""
	}

	if choices, ok := data["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := asMap(choices[0]); ok {
			if delta, ok := asMap(choice["delta"]); ok {
				if content := stringValue(delta["content"]); content != "" {
					return content
				}
			}
			if message, ok := asMap(choice["message"]); ok {
				if content := stringValue(message["content"]); content != "" {
					return content
				}
			}
			if text := stringValue(choice["text"]); text != "" {
				return text
			}
		}
	}

	if content := stringValue(data["content"]); content != "" {
		return content
	}
	if text := stringValue(data["text"]); text != "" {
		return text
	}

	return ""
}

// extractInteractionContent parses the final interaction body for a best-effort assistant response.
func extractInteractionContent(body []byte) string {
	var jsonMap map[string]interface{}
	if err := json.Unmarshal(body, &jsonMap); err != nil {
		return ""
	}

	if choices, ok := jsonMap["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := asMap(choices[0]); ok {
			if message, ok := asMap(choice["message"]); ok {
				if content := stringValue(message["content"]); content != "" {
					return content
				}
			}
			if text := stringValue(choice["text"]); text != "" {
				return text
			}
		}
	}

	if content := stringValue(jsonMap["content"]); content != "" {
		return content
	}
	if text := stringValue(jsonMap["text"]); text != "" {
		return text
	}

	return ""
}

// extractInteractionReasoningContent parses separately returned model reasoning.
func extractInteractionReasoningContent(body []byte) string {
	var jsonMap map[string]interface{}
	if err := json.Unmarshal(body, &jsonMap); err != nil {
		return ""
	}
	if choices, ok := jsonMap["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := asMap(choices[0]); ok {
			if message, ok := asMap(choice["message"]); ok {
				return stringValue(message["reasoning_content"])
			}
			return stringValue(choice["reasoning_content"])
		}
	}
	return stringValue(jsonMap["reasoning_content"])
}
