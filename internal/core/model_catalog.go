// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// listModelsTimeout bounds the HTTP request duration used by ListModels.
const listModelsTimeout = 30 * time.Second

// modelListResponse models the common {"data": [...]} response envelope.
// modelListResponse is an internal data type used by this package.
type modelListResponse struct {
	// Data stores the raw model entries from the server.
	Data []map[string]interface{} `json:"data"`
}

// modelRow is the normalized row rendered by the console table printer.
// modelRow is an internal data type used by this package.
type modelRow struct {
	// Name is the model identifier.
	Name string
	// Loaded reports whether the model is currently loaded.
	Loaded bool
	// Temperature stores the parsed temperature value.
	Temperature string
	// RepeatLastN stores the parsed repeat-last-n value.
	RepeatLastN string
	// RepeatPenalty stores the parsed repeat-penalty value.
	RepeatPenalty string
	// TopK stores the parsed top-k value.
	TopK string
	// TopP stores the parsed top-p value.
	TopP string
	// ContextSize stores the configured context window.
	ContextSize string
	// BatchSize stores the configured logical batch size.
	BatchSize string
	// UBatchSize stores the configured physical batch size.
	UBatchSize string
	// Parallel stores the configured slot count.
	Parallel string
	// CacheTypeK stores the key-cache quantization type.
	CacheTypeK string
	// CacheTypeV stores the value-cache quantization type.
	CacheTypeV string
	// FlashAttention reports the configured flash-attention mode.
	FlashAttention string
	// InputModalities stores the architecture input modalities.
	InputModalities string
	// OutputModalities stores the architecture output modalities.
	OutputModalities string
}

// ListModels fetches /v1/models from the provided endpoint and prints a table.
func ListModels(endpoint string, options ...ClientOption) error {
	client := NewClient(context.Background(), endpoint, options...)
	return client.ListModels()
}

// ListModels fetches /v1/models and sends its table to the configured logger.
func (c *Client) ListModels() error {
	ctx, cancel := context.WithTimeout(context.Background(), listModelsTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/v1/models", http.NoBody)
	if err != nil {
		return err
	}

	resp, err := c.clientHTTP().Do(req)
	if err != nil {
		return fmt.Errorf("models request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	models, err := decodeModelRows(resp.Body)
	if err != nil {
		return err
	}

	var table strings.Builder
	renderModelTable(&table, models)
	c.logTable(table.String())
	return nil
}

// logTable sends each table row through the logger separately so logger
// prefixes (timestamps, application names, and so on) have the same width on
// the header and every data row.
func (c *Client) logTable(table string) {
	for _, line := range strings.Split(strings.TrimRight(table, "\n"), "\n") {
		c.logf("%s", line)
	}
}

// clientHTTP returns the configured HTTP client, or a default client when missing.
func (c *Client) clientHTTP() *http.Client {
	if c != nil && c.opts != nil && c.opts.httpClient != nil {
		return c.opts.httpClient
	}
	return &http.Client{Timeout: 10 * time.Minute}
}

// decodeModelRows parses the /v1/models response into normalized table rows.
func decodeModelRows(r io.Reader) ([]modelRow, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read models body: %w", err)
	}

	var envelope modelListResponse
	if err := json.Unmarshal(body, &envelope); err == nil {
		rows := make([]modelRow, 0, len(envelope.Data))
		for _, item := range envelope.Data {
			rows = append(rows, modelRowFromMap(item))
		}
		return rows, nil
	}

	var raw []map[string]interface{}
	if err := json.Unmarshal(body, &raw); err == nil {
		rows := make([]modelRow, 0, len(raw))
		for _, item := range raw {
			rows = append(rows, modelRowFromMap(item))
		}
		return rows, nil
	}

	return nil, fmt.Errorf("failed to decode models json")
}

// renderModelTable prints a tabular model listing to the supplied writer.
func renderModelTable(w io.Writer, rows []modelRow) {
	headers := []string{"MODEL", "STATUS", "CTX", "BATCH", "UBATCH", "PARALLEL", "CACHE-K", "CACHE-V", "FLASH-ATTN", "TEMPERATURE", "TOP-K", "TOP-P", "REPEAT-LAST-N", "REPEAT-PENALTY", "INPUT MODALITIES", "OUTPUT MODALITIES"}
	values := make([][]string, 0, len(rows))
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}
	for _, row := range rows {
		loaded := "UNLOADED"
		if row.Loaded {
			loaded = "LOADED"
		}
		cols := []string{row.Name, loaded, row.ContextSize, row.BatchSize, row.UBatchSize, row.Parallel, row.CacheTypeK, row.CacheTypeV, row.FlashAttention, row.Temperature, row.TopK, row.TopP, row.RepeatLastN, row.RepeatPenalty, row.InputModalities, row.OutputModalities}
		for i, col := range cols {
			if len(col) > widths[i] {
				widths[i] = len(col)
			}
		}
		values = append(values, cols)
	}

	if len(values) == 0 {
		values = nil
	}

	formattedHeaders := make([]string, len(headers))
	for i, header := range headers {
		formattedHeaders[i] = padRight(header, widths[i])
	}
	_, _ = fmt.Fprintln(w, strings.Join(formattedHeaders, "  "))

	for _, cols := range values {
		formatted := make([]string, len(cols))
		for i, col := range cols {
			formatted[i] = padRight(col, widths[i])
		}
		_, _ = fmt.Fprintln(w, strings.Join(formatted, "  "))
	}
}

// padRight pads s with spaces until it reaches width characters.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// modelRowFromMap normalizes a raw model record into a printable row.
func modelRowFromMap(m map[string]interface{}) modelRow {
	arch := architectureValue(m)
	args := parsedModelArgs(extractModelArgs(m))
	return modelRow{
		Name:             valueOrDefault(firstNonEmptyValue(stringValue(m["id"]), stringValue(m["name"]))),
		Temperature:      valueOrDefault(firstNonEmptyValue(lookupModelValue(m, "temperature"), args["temperature"])),
		RepeatLastN:      valueOrDefault(firstNonEmptyValue(lookupModelValue(m, "repeat-last-n", "repeat_last_n"), args["repeat-last-n"])),
		RepeatPenalty:    valueOrDefault(firstNonEmptyValue(lookupModelValue(m, "repeat-penalty", "repeat_penalty"), args["repeat-penalty"])),
		TopK:             valueOrDefault(firstNonEmptyValue(lookupModelValue(m, "top-k", "top_k"), args["top-k"])),
		TopP:             valueOrDefault(firstNonEmptyValue(lookupModelValue(m, "top-p", "top_p"), args["top-p"])),
		ContextSize:      valueOrDefault(firstNonEmptyValue(lookupModelValue(m, "ctx-size", "ctx_size", "n_ctx"), args["ctx-size"])),
		BatchSize:        valueOrDefault(firstNonEmptyValue(lookupModelValue(m, "batch-size", "batch_size"), args["batch-size"])),
		UBatchSize:       valueOrDefault(firstNonEmptyValue(lookupModelValue(m, "ubatch-size", "ubatch_size"), args["ubatch-size"])),
		Parallel:         valueOrDefault(firstNonEmptyValue(lookupModelValue(m, "parallel"), args["parallel"])),
		CacheTypeK:       valueOrDefault(firstNonEmptyValue(lookupModelValue(m, "cache-type-k", "cache_type_k"), args["cache-type-k"])),
		CacheTypeV:       valueOrDefault(firstNonEmptyValue(lookupModelValue(m, "cache-type-v", "cache_type_v"), args["cache-type-v"])),
		FlashAttention:   valueOrDefault(firstNonEmptyValue(lookupModelValue(m, "flash-attn", "flash_attn"), args["flash-attn"])),
		InputModalities:  valueOrDefault(joinStringSlice(modalityStrings(arch, "input_modalities"))),
		OutputModalities: valueOrDefault(joinStringSlice(modalityStrings(arch, "output_modalities"))),
		Loaded:           isModelLoaded(m),
	}
}

// valueOrDefault makes omitted server settings explicit rather than rendering
// misleading empty table cells.
func valueOrDefault(value string) string {
	if value == "" {
		return "DEFAULT"
	}
	return value
}

// extractModelArgs searches a model record for a command-line argument list.
func extractModelArgs(m map[string]interface{}) interface{} {
	for _, key := range []string{"args", "arguments", "argv", "command", "cmd", "launch_args"} {
		if value, ok := m[key]; ok {
			return value
		}
	}

	for _, value := range m {
		if nested, ok := asMap(value); ok {
			if extracted := extractModelArgs(nested); extracted != nil {
				return extracted
			}
		}
	}

	return nil
}

// parsedModelArgs normalizes a command-line argument list into a flag-to-value map.
func parsedModelArgs(raw interface{}) map[string]string {
	args := argTokens(raw)
	if len(args) == 0 {
		return nil
	}

	values := make(map[string]string)
	for i := 0; i < len(args); i++ {
		token := strings.TrimSpace(args[i])
		if token == "" || !strings.HasPrefix(token, "--") {
			continue
		}

		flag := strings.TrimPrefix(token, "--")
		if i+1 < len(args) && !strings.HasPrefix(strings.TrimSpace(args[i+1]), "--") {
			values[flag] = strings.TrimSpace(args[i+1])
			i++
			continue
		}

		values[flag] = "true"
	}

	return values
}

// argTokens converts a raw command-line field into a token slice.
func argTokens(raw interface{}) []string {
	switch v := raw.(type) {
	case nil:
		return nil
	case string:
		return strings.Fields(v)
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, stringValue(item))
		}
		return out
	default:
		return stringSlice(v)
	}
}

// firstNonEmptyValue returns the first non-empty string from the supplied values.
func firstNonEmptyValue(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// lookupModelValue finds a value by trying multiple possible keys.
func lookupModelValue(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := nestedLookup(m, key); value != nil {
			return stringValue(value)
		}
	}
	return ""
}

// nestedLookup searches the top level and a couple of nested parameter maps.
func nestedLookup(m map[string]interface{}, key string) interface{} {
	if value, ok := m[key]; ok {
		return value
	}
	for _, parentKey := range []string{"parameters", "args"} {
		if nested, ok := asMap(m[parentKey]); ok {
			if value, ok := nested[key]; ok {
				return value
			}
		}
	}
	return nil
}

// architectureValue extracts the architecture sub-map when it exists.
func architectureValue(m map[string]interface{}) map[string]interface{} {
	if arch, ok := asMap(m["architecture"]); ok {
		return arch
	}
	return nil
}

// modalityStrings returns the string slice stored at the given architecture key.
func modalityStrings(m map[string]interface{}, key string) []string {
	if m == nil {
		return nil
	}
	if raw, ok := m[key]; ok {
		return stringSlice(raw)
	}
	return nil
}

// isModelLoaded checks whether a model record reports a loaded state.
func isModelLoaded(m map[string]interface{}) bool {
	if loaded, ok := m["loaded"].(bool); ok && loaded {
		return true
	}
	return modelStatusFromMap(m).State == ModelRuntimeLoaded
}

// asMap converts an interface value to a map when possible.
func asMap(v interface{}) (map[string]interface{}, bool) {
	if v == nil {
		return nil, false
	}
	m, ok := v.(map[string]interface{})
	return m, ok
}

// stringValue converts a loosely typed value into its string representation.
func stringValue(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case fmt.Stringer:
		return t.String()
	default:
		return fmt.Sprint(t)
	}
}

// stringSlice converts a loosely typed value into a slice of strings.
func stringSlice(v interface{}) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case []string:
		return t
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			out = append(out, stringValue(item))
		}
		return out
	default:
		return []string{stringValue(t)}
	}
}

// joinStringSlice joins non-empty strings using commas.
func joinStringSlice(values []string) string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			filtered = append(filtered, value)
		}
	}
	return strings.Join(filtered, ",")
}
