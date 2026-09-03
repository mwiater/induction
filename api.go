// Package induction provides a Go client and terminal-oriented helpers for
// llama.cpp-compatible inference servers, including chat, streaming, model
// inspection, telemetry snapshots, and MCP tool execution.
package induction

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// doInference handles the core completion or chat completion request.
func (c *Client) doInference(ctx context.Context, req *ChatRequest) (*Interaction, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Route to the correct endpoint based on whether the request is chat-shaped.
	endpoint := c.endpoint + "/v1/chat/completions"
	if len(req.Messages) == 0 {
		endpoint = c.endpoint + "/completion"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(payload))
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

	interaction := &Interaction{
		Response: string(bodyBytes),
	}
	interaction.Content = extractInteractionContent(bodyBytes)
	interaction.ReasoningContent = extractInteractionReasoningContent(bodyBytes)

	return interaction, nil
}

// infer sends an OpenAI-compatible request and decodes its response without
// collecting snapshot telemetry.
func (c *Client) infer(ctx context.Context, req *ChatRequest) (*InferenceResponse, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	path := "/v1/chat/completions"
	if len(req.Messages) == 0 {
		path = "/v1/completions"
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.clientHTTP().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("inference request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read inference body: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("inference request returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result InferenceResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode inference response: %w", err)
	}
	return &result, nil
}

// inferStreamChunks decodes an OpenAI-compatible SSE response one chunk at a time.
func (c *Client) inferStreamChunks(ctx context.Context, req *ChatRequest, yield func(InferenceStreamChunk) error) error {
	request := *req
	stream := true
	request.Stream = &stream
	payload, err := json.Marshal(&request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	path := "/v1/chat/completions"
	if len(request.Messages) == 0 {
		path = "/v1/completions"
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.clientHTTP().Do(httpReq)
	if err != nil {
		return fmt.Errorf("streaming request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("streaming request returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == doneStreamToken {
			return nil
		}

		var chunk InferenceStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("failed to decode inference stream chunk: %w", err)
		}
		if err := yield(chunk); err != nil {
			return fmt.Errorf("handle inference stream chunk: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read inference stream: %w", err)
	}
	return nil
}

// fetchProps gets model properties for the supplied model name.
func (c *Client) fetchProps(ctx context.Context, model string) (*PropsData, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/props", nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("model", model)
	req.URL.RawQuery = q.Encode()

	resp, err := c.clientHTTP().Do(req)
	if err != nil {
		return nil, fmt.Errorf("props request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read props body: %w", err)
	}

	props := &PropsData{Raw: string(body)}
	if err := json.Unmarshal(body, props); err != nil {
		return nil, fmt.Errorf("failed to decode props json: %w", err)
	}

	return props, nil
}

// fetchSlots gets slot telemetry for the supplied model name.
func (c *Client) fetchSlots(ctx context.Context, model string) (SlotsData, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/slots", nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("model", model)
	req.URL.RawQuery = q.Encode()

	resp, err := c.clientHTTP().Do(req)
	if err != nil {
		return nil, fmt.Errorf("slots request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var slots SlotsData
	if err := json.NewDecoder(resp.Body).Decode(&slots); err != nil {
		return nil, fmt.Errorf("failed to decode slots json: %w", err)
	}

	return slots, nil
}

// fetchMetrics gets the server metrics and parses Prometheus samples into a map.
func (c *Client) fetchMetrics(ctx context.Context, model string) (*MetricsData, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/metrics", nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("model", model)
	req.URL.RawQuery = q.Encode()

	resp, err := c.clientHTTP().Do(req)
	if err != nil {
		return nil, fmt.Errorf("metrics request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read metrics body: %w", err)
	}

	data := &MetricsData{
		Raw:     string(body),
		Entries: make(map[string]interface{}),
	}

	// Parse the Prometheus text format into the Entries map.
	scanner := bufio.NewScanner(strings.NewReader(data.Raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			if val, err := strconv.ParseFloat(parts[1], 64); err == nil {
				data.Entries[parts[0]] = val
			}
		}
	}

	return data, nil
}
