package induction

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

const mcpProtocolVersion = "2024-11-05"

type mcpClient struct {
	url        string
	httpClient *http.Client
	mu         sync.Mutex
	nextID     int64
	sessionID  string
}

type mcpTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations struct {
		ReadOnlyHint bool `json:"readOnlyHint"`
	} `json:"annotations,omitempty"`
}

type mcpCallResult struct {
	Content           []mcpContent    `json:"content,omitempty"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
	IsError           bool            `json:"isError,omitempty"`
}

type mcpContent struct {
	Type string          `json:"type"`
	Text string          `json:"text,omitempty"`
	Data json.RawMessage `json:"-"`
}

func (c *mcpContent) UnmarshalJSON(data []byte) error {
	type content mcpContent
	if err := json.Unmarshal(data, (*content)(c)); err != nil {
		return err
	}
	c.Data = append(c.Data[:0], data...)
	return nil
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data,omitempty"`
	} `json:"error,omitempty"`
}

func (c *mcpClient) initialize(ctx context.Context) error {
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	err := c.request(ctx, "initialize", map[string]any{
		"protocolVersion": mcpProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": "induction-infer-mcp", "version": "1.0.0"},
	}, &result)
	if err != nil {
		return fmt.Errorf("initialize MCP session: %w", err)
	}
	if result.ProtocolVersion == "" {
		return errors.New("initialize MCP session: server omitted protocolVersion")
	}
	if err := c.notify(ctx, "notifications/initialized", map[string]any{}); err != nil {
		return fmt.Errorf("notify MCP initialized: %w", err)
	}
	return nil
}

func (c *mcpClient) listTools(ctx context.Context) ([]mcpTool, error) {
	var all []mcpTool
	var cursor string
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var page struct {
			Tools      []mcpTool `json:"tools"`
			NextCursor string    `json:"nextCursor,omitempty"`
		}
		if err := c.request(ctx, "tools/list", params, &page); err != nil {
			return nil, fmt.Errorf("list MCP tools: %w", err)
		}
		all = append(all, page.Tools...)
		if page.NextCursor == "" {
			return all, nil
		}
		cursor = page.NextCursor
	}
}

func (c *mcpClient) callTool(ctx context.Context, name string, arguments json.RawMessage) (mcpCallResult, error) {
	var args any = map[string]any{}
	if len(arguments) > 0 {
		if err := json.Unmarshal(arguments, &args); err != nil {
			return mcpCallResult{}, fmt.Errorf("decode arguments for %q: %w", name, err)
		}
	}
	var result mcpCallResult
	if err := c.request(ctx, "tools/call", map[string]any{"name": name, "arguments": args}, &result); err != nil {
		return mcpCallResult{}, fmt.Errorf("call MCP tool %q: %w", name, err)
	}
	return result, nil
}

func (c *mcpClient) notify(ctx context.Context, method string, params any) error {
	return c.send(ctx, map[string]any{"jsonrpc": "2.0", "method": method, "params": params}, nil)
}

func (c *mcpClient) request(ctx context.Context, method string, params any, result any) error {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()
	return c.send(ctx, map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}, result)
}

func (c *mcpClient) send(ctx context.Context, payload any, result any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	c.mu.Lock()
	sessionID := c.sessionID
	c.mu.Unlock()
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if id := resp.Header.Get("Mcp-Session-Id"); id != "" {
		c.mu.Lock()
		c.sessionID = id
		c.mu.Unlock()
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if result == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		data, err = lastSSEData(data)
		if err != nil {
			return err
		}
	}
	var rpc rpcResponse
	if err := json.Unmarshal(data, &rpc); err != nil {
		return fmt.Errorf("decode JSON-RPC response: %w", err)
	}
	if rpc.Error != nil {
		return fmt.Errorf("MCP error %d: %s", rpc.Error.Code, rpc.Error.Message)
	}
	if err := json.Unmarshal(rpc.Result, result); err != nil {
		return fmt.Errorf("decode MCP result: %w", err)
	}
	return nil
}

func lastSSEData(body []byte) ([]byte, error) {
	var data []byte
	s := bufio.NewScanner(bytes.NewReader(body))
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "data:") {
			data = append([]byte(nil), strings.TrimSpace(strings.TrimPrefix(line, "data:"))...)
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("MCP SSE response contained no data event")
	}
	return data, nil
}
